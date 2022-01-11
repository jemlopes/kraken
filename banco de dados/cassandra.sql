DROP KEYSPACE neurose_kraken;
CREATE KEYSPACE neurose_kraken  WITH REPLICATION = { 'class' : 'SimpleStrategy', 'replication_factor' : 1 };
    
use neurose_kraken;	
  
CREATE TABLE orders ( id text ,order_number text, reference text, status int, created_at timestamp,  updated_at timestamp , paid_amount int , total_amount int, PRIMARY KEY (id) );

CREATE TABLE orderitems ( id text , order_id text, sku text, quantity int, unit_price int, total_price int, created_at timestamp , PRIMARY KEY (order_id, id)) ;

CREATE TABLE transactions (  id text,  order_id text, created_at timestamp,  updated_at timestamp,  amount int , transaction_type int , PRIMARY KEY (order_id, id))  ;





