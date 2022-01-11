package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gocql/gocql"
	"github.com/kataras/iris"
	"github.com/valyala/fasthttp"
)

const (
	version = "   Kraken Release: 1.0.0_08"

	banner = `          
            /\          
         _ /  \ _
         \/"\/"\/ 
         / \  / \
         |(@)(@)|
         )  __  (
        //'))(( \\
       (( ((  )) ))        
        \\ ))(( //
		`
)

type ordersAPI struct {
	*iris.Context
}

type orderAPI struct {
	*iris.Context
}

type orderItemsAPI struct {
	*iris.Context
}

type orderItemAPI struct {
	*iris.Context
}

type transactionsAPI struct {
	*iris.Context
}

type transactionAPI struct {
	*iris.Context
}

type defaultAPI struct {
	*iris.Context
}

type consistencyAPI struct {
	*iris.Context
}

var config = getSettings()
var Session *gocql.Session
var Cache Kache
var mainURI string
var repo Repository
var SystemLocation *time.Location

func main() {
	var err error
	SystemLocation, _ := time.LoadLocation("Brazil/East")
	StartLogging(SystemLocation)
	repo = Repository{}

	var points []string
	if strings.Index(config.Cassandra.ContactPoints, ",") > 0 {
		points = strings.Split(config.Cassandra.ContactPoints, ",")
	} else {
		points = []string{config.Cassandra.ContactPoints}
	}
	cluster := gocql.NewCluster(points...)
	//cluster.Compressor = gocql.SnappyCompressor{}

	cluster.ProtoVersion = config.Cassandra.Protoversion
	cluster.Keyspace = config.Cassandra.Keyspace

	//cluster.CQLVersion = config.Cassandra.Cqlversion

	cluster.NumConns = config.Cassandra.Connections

	Session, err = cluster.CreateSession()

	if err != nil {
		iris.Logger.Fatalf("nao foi possivel conectar ao cassandra: %s", err)
		return
	}

		if config.Cassandra.EnableFullConsistency {
		Session.SetConsistency(gocql.All)
	}

	fmt.Printf("Kraken Configuration:\n%v\n\n", config)
	if config.Cache.EnableCache {
		Cache.StartCache()
	}
	setupIris()

	defer Session.Close()
}

/*Web API funcions */
func setupIris() {
	/*set up iris */
	logc.Infof("%s", version)
	logc.Infof("Using IRIS %s\n", iris.Version)

	iris.Config.DisableBanner = true
	mainURI = "/" + config.App.ApiVersion

	iris.Logger.PrintBanner(banner, version)

	/*Default*/
	iris.API("/", defaultAPI{})
	/*D-cache*/
	iris.API("/synccache/:action", defaultAPI{})
	/* Consitency checks */
	iris.API("/consistency/listolastorders/:begin/:end/:count", consistencyAPI{})
	iris.API("/consistency/listrangeorders/:begin/:end", consistencyAPI{})
	/*Order */
	iris.API(mainURI+"/orders", ordersAPI{})
	iris.API(mainURI+"/orders/:orderId", orderAPI{})
	iris.API(mainURI+"/orders/:orderId/perform/:action", orderAPI{})
	/*Order Items */
	iris.API(mainURI+"/orders/:orderId/items", orderItemsAPI{})
	iris.API(mainURI+"/orders/:orderId/items/:orderItemId", orderItemAPI{})
	/*Transactions*/
	iris.API(mainURI+"/orders/:orderId/transactions", transactionsAPI{})
	iris.API(mainURI+"/orders/:orderId/transactions/:transactionId", transactionAPI{})

	iris.Listen(config.App.Address + ":" + config.App.Port)
}

/*Common API */

/*AWS Health Check*/
func (api defaultAPI) Get() {
	api.SetStatusCode(iris.StatusOK)
}

/* Cache integration API */
func (api defaultAPI) Post() {
	action := api.Param("action")
	if (action == "u" || action == "d") && config.Cache.UseDistributed {
		contentRaw := api.Request.Body()
		content := []byte{}
		content, err := fasthttp.AppendGunzipBytes(content, contentRaw)
		serverinfo := strings.Split(api.RequestCtx.RemoteAddr().String(), ":")
		if config.Cache.EnableCache && config.Cache.UseDistributed {

			if !strings.Contains(config.Cache.DistributionPoints, serverinfo[0]) {
				logc.Errorf("RcacheSync Unauthorized Server: <%s>", serverinfo[0])
				api.JSON(iris.StatusUnauthorized, "")
			}

			if err != nil {
				logc.Errorf("RcacheSync Error: <%s>", err)
				api.JSON(iris.StatusBadRequest, err)
			}

			logc.Debugf("RCVCACHE: %s :: content: <%s> from server <%s>", action, content, serverinfo[0])
			go Cache.ReceiveBundle(content, action)
		} else {
			api.JSON(iris.StatusBadRequest, "invalid request")
		}
	} else {
		api.JSON(iris.StatusForbidden, "cache disabled for server")
	}
}

/* Consistency checks API */

func (api consistencyAPI) Get() {
	orders, err := repo.ReadOrdersByRange(api.Param("begin"), api.Param("end"))
	if err != nil {
		api.JSON(iris.StatusInternalServerError, err)
	} else {
		api.JSON(iris.StatusOK, orders)
	}
}

/* Orders REST API */

func (api ordersAPI) Post() {
	order := Order{}
	err := api.ReadJSON(&order)

	if err != nil {
		logc.Errorf("%s", err)
		api.JSON(iris.StatusBadRequest, err)
		return
	}

	order, err = repo.CreateOrder(order)

	if err != nil {
		api.JSON(iris.StatusInternalServerError, err)
	} else {
		api.SetHeader("Location", fmt.Sprintf("%s/orders/%s", mainURI, order.ID))
		api.SetStatusCode(iris.StatusCreated)
		api.SetBodyString("{\"id\": \"" + order.ID + "\"}")
	}
}

func (api orderAPI) Get() {
	order, err := repo.ReadOrder(api.Param("orderId"))
	if err != nil {
		api.JSON(iris.StatusInternalServerError, err)
	} else {
		api.JSON(iris.StatusOK, order)
	}
}

func (api orderAPI) Put() {

	order, err := repo.ReadOrder(api.Param("orderId"))
	if err == nil {
		action := strings.ToLower(api.Param("action"))

		if action == "close" {
			order.Status = ORDER_CLOSED
		} else if action == "cancel" {
			order.Status = ORDER_CANCELED
		} else {
			api.SetStatusCode(412)
			return
		}
		repo.UpdateOrderStatus(order)

		api.JSON(iris.StatusOK, order)
	} else {
		api.JSON(iris.StatusInternalServerError, err)
	}
}

/* Orders Items REST API */

func (api orderItemsAPI) Post() {
	orderItem := OrderItem{}
	err := api.ReadJSON(&orderItem)
	if err != nil {
		logc.Errorf("%s", err)
		api.JSON(iris.StatusBadRequest, err)
		return
	}

	orderItem.OrderID = api.Param("orderId")
	orderItem.TotalPrice = orderItem.Quantity * orderItem.UnitPrice

	err = orderItem.Validate()
	if err != nil {
		api.SetStatusCode(iris.StatusBadRequest)
		api.SetBodyString(err.Error())

	} else {
		orderItem, err = repo.CreateOrderItem(orderItem)
		if err != nil {
			api.SetStatusCode(iris.StatusInternalServerError)
			api.SetBodyString(err.Error())
		} else {
			api.SetHeader("Location", fmt.Sprintf("%s/orders/%s/items/%s", mainURI, orderItem.OrderID, orderItem.ID))
			api.JSON(iris.StatusCreated, orderItem)
		}
	}
}

func (api orderItemAPI) Get() {
	oi, err := repo.ReadOrder(api.Param("orderItemId"))
	if err != nil {
		api.JSON(iris.StatusInternalServerError, err)
	} else {
		api.JSON(iris.StatusOK, oi)
	}
}

/* Tramsactions REST API */

func (api transactionsAPI) Post() {
	transaction := Transaction{}
	err := api.ReadJSON(&transaction)
	if err != nil {
		logc.Errorf("%s", err)
		api.JSON(iris.StatusBadRequest, err)
		return
	}

	transaction.OrderID = api.Param("orderId")
	transaction, err = repo.CreateTransaction(transaction)
	if err != nil {
		api.SetStatusCode(iris.StatusInternalServerError)
		api.SetBodyString(err.Error())
	} else {
		api.SetHeader("Location", fmt.Sprintf("%s/orders/%s/transactions/%s", mainURI, transaction.OrderID, transaction.ID))
		api.JSON(iris.StatusCreated, transaction)
	}
}

func (api transactionAPI) Get() {
	t, err := repo.ReadOrder(api.Param("transactionId"))
	if err != nil {
		api.JSON(iris.StatusInternalServerError, err)
	} else {
		api.JSON(iris.StatusOK, t)
	}
}
