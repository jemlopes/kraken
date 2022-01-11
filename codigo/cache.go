package main

import (
	"container/heap"
	"encoding/json"
	"strings"
	"time"

	"github.com/streamrail/concurrent-map"
	"github.com/valyala/fasthttp"
)

type Cacheable interface {
	cacheID() string
	timeStamp() time.Time
}

type Kache struct {
	cacheBag       cmap.ConcurrentMap
	cacheMeta      cmap.ConcurrentMap
	servers        map[string]bool
	connectionPool []bool
}

type Connection struct {
	isAvailable bool
}

var bheap = &broadcastHeap{}

var cacheLocation, _ = time.LoadLocation("Brazil/East")

func (c *Kache) StartCache() {
	c.cacheBag = cmap.New()
	c.cacheMeta = cmap.New()
	c.servers = make(map[string]bool)
	c.connectionPool = make([]bool, config.Cache.DistributedPooling)
	heap.Init(bheap)
	var configServers []string
	if strings.Index(config.Cache.DistributionPoints, ",") > 0 {
		configServers = strings.Split(config.Cache.DistributionPoints, ",")
	} else {
		configServers = []string{config.Cache.DistributionPoints}
	}

	for i := 0; i < len(configServers); i++ {
		c.servers[configServers[i]] = true
	}

	for i := 0; i < config.Cache.DistributedPooling; i++ {
		c.connectionPool[i] = true
	}

	go c.cacheManager()

	go c.broadcast()

}

func (c *Kache) Save(o Order) error {
	var err error
	c.save(o)
	if config.Cache.UseDistributed {
		go c.sendToBroadcast(o.ID)
	}
	return err
}

func (c *Kache) save(o Order) error {
	logc.Debugf("Saving in Cache: %s %v", o.ID, o.Items)
	c.cacheBag.Set(o.ID, o)
	c.cacheMeta.Set(o.ID, time.Now().In(cacheLocation))
	return nil
}

func (c *Kache) Remove(key string) {
	c.cacheBag.Remove(key)
}

func (c *Kache) Get(key string) (Order, bool) {
	order := Order{}
	ok := false
	cacheItem, ok := c.cacheBag.Get(key)
	if ok {
		order = cacheItem.(Order)
	}
	return order, ok
}

func (c *Kache) GetJson(key string) ([]byte, bool) {
	orderj := []byte{}

	order, ok := c.cacheBag.Get(key)
	if ok {
		orderj, _ = json.Marshal(&order)
	} else {
		logc.Errorf("Not found in Cache(Json):  %s", key)
	}

	return orderj, ok
}

func (c *Kache) cacheManager() {
	var duration time.Duration
	for {
		time.Sleep(time.Duration(config.Cache.UpdateIntervalinMillis) * time.Millisecond)
		logc.Debugf("Cache loop:")
		logc.Debugf("Items in Cache: %d", c.cacheBag.Count())
		for item := range c.cacheMeta.Iter() {
			duration = time.Since(item.Val.(time.Time))
			logc.Debugf("Check: %s Times: %f vs %f (should delete? %t )", item.Key, duration.Minutes(), float64(config.Cache.ExpirationTimeinMinutes), duration.Minutes() > float64(config.Cache.ExpirationTimeinMinutes))
			if duration.Minutes() > float64(config.Cache.ExpirationTimeinMinutes) {
				logc.Debugf("Removing %s\n", item.Key)
				c.Remove(item.Key)
			}
		}

		logc.Debugf("Updated Items in Cache: %d", c.cacheBag.Count())
	}
}

func (c *Kache) sendToBroadcast(key string) {
	heap.Push(bheap, key)
}

func (c *Kache) broadcast() {
	noOrdersMax := 1000
	for {
		time.Sleep(time.Duration(config.Cache.BroadCastTimeInMillis) * time.Millisecond)
		noOrders := bheap.Len()

		if noOrders > 0 {
			if noOrders > noOrdersMax {
				noOrders = noOrdersMax
			}
			orders := []Order{}
			//	orders := []byte{}
			ordersKeys := bheap.PopMultiple(noOrders)
			for i := 0; i < len(ordersKeys); i++ {
				order, _ := c.Get(ordersKeys[i])
				orders = append(orders, order)
				logc.Debugf("Broadcasting -> Order: %s with items %v", order.ID, order.Items)
			}
			ordersJSON, _ := json.Marshal(orders)
			content := []byte{}
			content = fasthttp.AppendGzipBytes(content, ordersJSON)
			for host, send := range c.servers {
				if send {
					j := 0
					for {
						if c.connectionPool[j] == true {
							go c.broadcastToServer(j, host, content)
							break
						}
						j++
						if j >= config.Cache.DistributedPooling {
							j = 0
						}
					}
				}
			}
		}

	}
}

func (c *Kache) ReceiveBundle(bOrders []byte, action string) {
	orders := []Order{}
	err := json.Unmarshal(bOrders, &orders)
	//Parse orders, reject stale cache info
	logc.Debugf("Receiving cache bundle. Items to process= %d , %v ", len(orders), err)
	for i := 0; i < len(orders); i++ {
		o := orders[i]
		u := false
		oc, exists := c.Get(o.ID)
		if exists {
			tn := time.Since(o.UpdatedAt)
			to := time.Since(oc.UpdatedAt)
			u = tn < to
		} else {
			u = true
		}
		if u {
			//Update orderID from item since json doesnt save this info
			logc.Debugf("Updating cache item (%s)...", o.ID)
			for j := 0; j < len(o.Items); j++ {
				o.Items[j].OrderID = o.ID
			}
			//Update orderID from transaction since json doesnt save this info
			logc.Debugf("Updating cache transaction (%s)...", o.ID)
			for j := 0; j < len(o.Transactions); j++ {
				o.Transactions[j].OrderID = o.ID
			}
			c.save(o)
		} else {
			logc.Debugf("cache item (%s) is stale. Discarding...", o.ID)
		}
	}
}

func (c *Kache) broadcastToServer(conn int, host string, content []byte) error {
	c.connectionPool[conn] = false
	server := "http://" + host + ":" + config.App.Port + "/synccache/u"
	connector := &fasthttp.Client{
		Name:         host,
		WriteTimeout: 100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	req.Header.SetMethodBytes([]byte("POST"))
	req.Header.SetContentTypeBytes([]byte("application/kache"))
	req.SetRequestURI(server)
	req.AppendBody(content)

	resp.SkipBody = true
	logc.Debugf("start sending cache: %s", server)
	err := connector.DoTimeout(req, resp, 500*time.Millisecond)
	if err != nil {
		logc.Errorf("Error sending cache: %s. Disabling cache for server: %s", err, host)
	}
	if resp.Header.StatusCode() != fasthttp.StatusOK {
		logc.Errorf("Unexpected status code: %d. Expecting %d", resp.Header.StatusCode(), fasthttp.StatusOK)
	}
	logc.Debugf("end sending cache: %s", server)
	fasthttp.ReleaseRequest(req)
	fasthttp.ReleaseResponse(resp)
	c.connectionPool[conn] = true
	return err
}
