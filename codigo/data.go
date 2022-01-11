package main

import (
	"errors"
	"sort"
	"time"

	"github.com/satori/go.uuid"
)

type OrderStatus int
type TransactionType int
type Transactions []Transaction

const (
	ORDER_DRAFT OrderStatus = iota
	ORDER_ENTERED
	ORDER_CANCELED
	ORDER_PAID
	ORDER_APPROVED
	ORDER_REJECTED
	ORDER_RE_ENTERED
	ORDER_CLOSED
)

const (
	TRANSACTION_PAYMENT TransactionType = iota
	TRANSACTION_CANCEL
	TRANSACTION_REFUND
)

type Order struct {
	ID           string        `json:"id"`
	Reference    string        `json:"reference"`
	Number       string        `json:"number"`
	Status       OrderStatus   `json:"status"`
	CreatedAt    time.Time     `json:"createdAt"`
	UpdatedAt    time.Time     `json:"lastUpdate"`
	PaidAmount   int           `json:"paidAmount"`
	TotalAmount  int           `json:"totalAmount"`
	Items        []OrderItem   `json:"items"`
	Transactions []Transaction `json:"transactions"`
}

type OrderItem struct {
	ID         string    `json:"id"`
	OrderID    string    `json:"-"`
	Sku        string    `json:"sku"`
	UnitPrice  int       `json:"unitPrice"`
	TotalPrice int       `json:"totalPrice"`
	Quantity   int       `json:"quantity"`
	CreatedAt  time.Time `json:"-"`
}

type Transaction struct {
	ID        string          `json:"id"`
	OrderID   string          `json:"-"`
	Type      TransactionType `json:"type"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
	Amount    int             `json:"amount"`
}

var systemLocation, _ = time.LoadLocation("Brazil/East")

/*Validations */

func (oi *OrderItem) Validate() error {
	if oi.Quantity < 0 {
		logc.Errorf("Quantidade nao pode ser menor que 1")
		return errors.New("Quantidade nao pode ser menor que 1")
	}

	return nil
}

func (t *Transaction) Validate() error {
	if t.OrderID == "" {
		logc.Errorf("OrderID nao pode ser nulo")
		return errors.New("OrderID nao pode ser nulo")
	}

	if t.Amount == 0 {
		logc.Errorf("Valor nao pode ser nulo")
		return errors.New("Valor nao pode ser nulo")
	}

	return nil
}

type Repository struct{}

/* Database functions */

/* Orders */

func (r *Repository) CreateOrder(o Order) (Order, error) {
	o.ID = uuid.NewV4().String()
	o.Status = ORDER_ENTERED

	o.CreatedAt = time.Now().In(systemLocation)
	o.UpdatedAt = o.CreatedAt

	o.TotalAmount = 0
	o.PaidAmount = 0

	if err := Session.Query("INSERT INTO orders (id , order_number, reference, status, created_at, updated_at) VALUES (?,?,?,?,?,?)", o.ID, o.Number, o.Reference, o.Status, o.CreatedAt, o.UpdatedAt).Exec(); err != nil {
		logc.Errorf("CreateOrder:: %s ::: %s", o.ID, err)
		return o, err
	}

	if config.Cache.EnableCache {
		go Cache.Save(o)
	}

	return o, nil
}

func (r *Repository) ReadOrder(id string) (Order, error) {
	o := Order{}
	cacheOk := false
	var err error
	if config.Cache.EnableCache {
		o, cacheOk = Cache.Get(id)
	}

	if !cacheOk {
		o, err = r.ReadFromDb(id)
		if config.Cache.EnableCache && err == nil {
			go Cache.Save(o)
		}
	}

	return o, err
}

func (r *Repository) ReadFromDb(id string) (Order, error) {
	o := Order{}
	if err := Session.Query("SELECT * FROM orders WHERE id = ?", id).Scan(&o.ID, &o.CreatedAt, &o.Number, &o.PaidAmount, &o.Reference, &o.Status, &o.TotalAmount, &o.UpdatedAt); err != nil {
		logc.Errorf("Order::ReadFromDb::Order:: %s ::: %s", id, err)
		return o, err
	}
	o.CreatedAt = o.CreatedAt.In(systemLocation)
	o.UpdatedAt = o.UpdatedAt.In(systemLocation)
	itemsOk := make(chan bool)
	transactionsOk := make(chan bool)
	go r.readOrderItems(&o, itemsOk)
	go r.readOrderTransactions(&o, transactionsOk)
	<-itemsOk
	<-transactionsOk

	if (o.Status != ORDER_CANCELED) && (o.Status != ORDER_CLOSED) {
		go r.updateOrder(o)
	}

	if config.Cache.EnableCache {
		go Cache.Save(o)
	}

	return o, nil
}

func (r *Repository) UpdateOrderStatus(o Order) error {
	o.UpdatedAt = time.Now().In(systemLocation)
	if err := Session.Query("UPDATE orders SET status = ? , updated_at=? WHERE id=?", o.Status, o.UpdatedAt, o.ID).Exec(); err != nil {
		logc.Errorf("Update::Order:: %s ::: %s", o.ID, err)
		return err
	}

	if config.Cache.EnableCache {
		go Cache.Save(o)
	}
	return nil
}

/*Consistency checks */

func copyOrder(o1 *Order, o2 *Order) {
	*o1 = *o2
}

func copyOrderItem(o1 *OrderItem, o2 *OrderItem) {
	*o1 = *o2
}

func copyTransaction(o1 *Transaction, o2 *Transaction) {
	*o1 = *o2
}

func (r *Repository) ReadOrdersByRangeLimited(strBegin string, strEnd string, limit int) ([]Order, error) {
	var orders []Order
	//Default time format template: Mon Jan 2 15:04:05 MST 2006
	beginRange, err := time.Parse("200601021504", strBegin)
	endRange, err := time.Parse("200601021504", strEnd)
	if err != nil {
		logc.Errorf("ReadOrdersByRangeLimited::Invalid Begin Date: %s", err)
		return nil, err
	}
	if err != nil {
		logc.Errorf("ReadOrdersByRangeLimited::Invalid End Date: %s", err)
	}

	iter := Session.Query("SELECT * FROM orders WHERE updated_at >= ? and updated_at <= ? LIMIT ?  ALLOW FILTERING", beginRange, endRange, limit).Iter()
	ot := Order{}
	for iter.Scan(&ot.ID, &ot.CreatedAt, &ot.Number, &ot.PaidAmount, &ot.Reference, &ot.Status, &ot.TotalAmount, &ot.UpdatedAt) {
		o := Order{}
		copyOrder(&o, &ot)
		o.CreatedAt = o.CreatedAt.In(systemLocation)
		o.UpdatedAt = o.UpdatedAt.In(systemLocation)
		itemsOk := make(chan bool)
		transactionsOk := make(chan bool)
		go r.readOrderItems(&o, itemsOk)
		go r.readOrderTransactions(&o, transactionsOk)
		<-itemsOk
		<-transactionsOk
		orders = append(orders, o)
	}
	if err := iter.Close(); err != nil {
		logc.Errorf("Orders::readOrderRange::Items:: %s", err)
	}
	return orders, nil
}
func (r *Repository) ReadOrdersByRange(strBegin string, strEnd string) ([]Order, error) {
	return r.ReadOrdersByRangeLimited(strBegin, strEnd, 2000000000)
}

/* Internal Order functions */
func (r *Repository) updateOrder(o Order) {
	o.UpdatedAt = time.Now().In(systemLocation)
	if err := Session.Query("UPDATE orders SET paid_amount = ? , total_amount = ? , updated_at=? WHERE id=?", o.PaidAmount, o.TotalAmount, o.UpdatedAt, o.ID).Exec(); err != nil {
		logc.Errorf("Update::Order:: %s ::: %s", o.ID, err)
	}
}

func (r *Repository) readOrderItems(o *Order, ok chan bool) {
	oi := OrderItem{}
	o.TotalAmount = 0
	iter := Session.Query("SELECT * from orderitems where order_id= ? ", o.ID).Iter()
	for iter.Scan(&oi.OrderID, &oi.ID, &oi.CreatedAt, &oi.Quantity, &oi.Sku, &oi.TotalPrice, &oi.UnitPrice) {
		oib := OrderItem{}
		copyOrderItem(&oib, &oi)
		oib.OrderID = o.ID
		o.Items = append(o.Items, oib)
		o.TotalAmount += oi.TotalPrice
	}
	if err := iter.Close(); err != nil {
		logc.Errorf("Order::readOrderItems::Items:: %s ::: %s", o.ID, err)
	}

	ok <- true
}

func (r *Repository) readOrderTransactions(o *Order, ok chan bool) {
	t := Transaction{}
	ts := Transactions{}
	o.PaidAmount = 0
	iter := Session.Query("SELECT * from transactions where order_id= ? ", o.ID).Iter()
	for iter.Scan(&t.OrderID, &t.ID, &t.Amount, &t.CreatedAt, &t.Type, &t.UpdatedAt) {
		tb := Transaction{}
		copyTransaction(&tb, &t)
		tb.CreatedAt = tb.CreatedAt.In(systemLocation)
		tb.UpdatedAt = tb.UpdatedAt.In(systemLocation)
		ts = append(ts, tb)
		o.PaidAmount += tb.Amount
	}
	if err := iter.Close(); err != nil {
		logc.Errorf("Order::readOrderTransacations::Transactions:: %s ::: %s", o.ID, err)
	}
	//Sort results by date
	sort.Sort(ts)
	if ts.Len() > 0 {
		o.UpdatedAt = ts[ts.Len()-1].UpdatedAt
	}
	o.Transactions = ts
	ok <- true
}

/* Orders - Cache functions */

func (r *Repository) addOrderItemCache(oi OrderItem) error {
	o, err := r.ReadOrder(oi.OrderID)
	if err == nil {
		o.TotalAmount += oi.TotalPrice
		oi.OrderID = o.ID
		o.Items = append(o.Items, oi)
		Cache.Save(o)
	}
	return nil
}

func (r *Repository) removeOrderItemCache(oi OrderItem) error {
	o, err := r.ReadOrder(oi.OrderID)
	if err == nil {
		var idx = 0
		for i := 0; i < len(o.Items); i++ {
			if o.Items[i].ID == oi.ID {
				idx = i
			}
		}
		o.TotalAmount -= oi.TotalPrice
		o.Items = append(o.Items[:idx], o.Items[idx+1:]...)
		Cache.Save(o)
	}
	return nil
}

func (r *Repository) addTransactionCache(t Transaction) error {
	o, err := r.ReadOrder(t.OrderID)
	if err == nil {
		o.PaidAmount += t.Amount
		t.OrderID = o.ID
		o.Transactions = append(o.Transactions, t)
		Cache.Save(o)
	}
	return nil
}

/* Order Items */

func (r *Repository) ReadOrderItem(id string, orderID string) (OrderItem, error) {
	oi := OrderItem{}
	err := Session.Query("SELECT * FROM orderitems WHERE order_id = ? and id = ? ", id, orderID).Scan(&oi.OrderID, &oi.ID, &oi.CreatedAt, &oi.Quantity, &oi.Sku, &oi.TotalPrice, &oi.UnitPrice)
	if err != nil {
		logc.Errorf("Read::OrderItem:: %s ::: %s", id, err)
	}
	return oi, err
}

func (r *Repository) CreateOrderItem(oi OrderItem) (OrderItem, error) {
	oi.ID = uuid.NewV4().String()
	oi.CreatedAt = time.Now().In(systemLocation)
	err := Session.Query("INSERT INTO orderitems (id, order_id, sku , quantity, unit_price, total_price , created_at) VALUES (?,?,?,?,?,?,?)", oi.ID, oi.OrderID, oi.Sku, oi.Quantity, oi.UnitPrice, oi.TotalPrice, oi.CreatedAt).Exec()
	if err != nil {
		logc.Errorf("Create::Item:: %s %s ::: %s", oi.OrderID, oi.ID, err)
	}
	if config.Cache.EnableCache {
		r.addOrderItemCache(oi)
	}
	return oi, err
}

/* Transactions */

func (r *Repository) ReadTransaction(id string) (Transaction, error) {
	t := Transaction{}
	err := Session.Query("SELECT * FROM transactions WHERE id = ? and order_id = ? order by created_at", id, t.OrderID).Scan(&t.OrderID, &t.ID, &t.Amount, &t.CreatedAt, &t.Type, &t.UpdatedAt)
	if err != nil {
		logc.Errorf("Read::Transaction::UpdatePayment:: %s %s ::: %s", t.OrderID, t.ID, err)
	}
	return t, err
}

func (r *Repository) CreateTransaction(t Transaction) (Transaction, error) {
	t.ID = uuid.NewV4().String()
	t.CreatedAt = time.Now().In(systemLocation)
	t.UpdatedAt = t.CreatedAt
	err := Session.Query("INSERT INTO transactions (id, order_id, transaction_type, created_at, updated_at, amount ) VALUES (?,?,?,?,?,?)", t.ID, t.OrderID, t.Type, t.CreatedAt, t.UpdatedAt, t.Amount).Exec()
	if err != nil {
		logc.Errorf("Create::Transaction:: %s %s ::: %s", t.OrderID, t.ID, err)
	} else {
		if config.Cache.EnableCache {
			go r.addTransactionCache(t)
		}
	}
	return t, err
}

func (slice Transactions) Len() int {
	return len(slice)
}

func (slice Transactions) Less(i, j int) bool {
	return slice[i].CreatedAt.Before(slice[j].CreatedAt)
}

func (slice Transactions) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}
