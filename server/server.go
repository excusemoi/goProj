package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"goProj/cache"
	"goProj/config"
	"goProj/dataFactory"
	"goProj/db"
	"goProj/natsImp/subscriber"
	"log"
	"net/http"
	"os"
)

type Server struct {
	cfg 			*config.Config
	db 				*sqlx.DB
	cache 			*cache.Cache
	serverPort 		string
	pathToCfg		string
}

func InitServer() (*Server, error){
	if len(os.Args) < 2 {
		return nil, errors.New("Please indicate the path to the config file")
	}
	pathToCfg := os.Args[1]
	cfg := config.Get(pathToCfg)
	pgDb, err := db.Dial(cfg)
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:        cfg,
		db:         pgDb,
		cache:      cache.InitCache(),
		serverPort: "3000",
		pathToCfg:  pathToCfg,
	}, nil
}

func (s *Server) Run() error {
	fmt.Println("Listening on port :" + s.serverPort)

	http.HandleFunc("/", s.HandleFunction)

	err := s.TryToRestore()

	if err != nil {
		return err
	}

	err = subscriber.Run(s.pathToCfg)

	if err != nil {
		return err
	}

	if err = http.ListenAndServe(":" + s.serverPort, nil); err != nil{
		return err
	}

	return nil
}

func (s *Server) HandleFunction(writer http.ResponseWriter, request *http.Request){
	switch request.Method {
	case "GET":
		s.get(writer, request)
	case "POST":
		if err := s.post(writer, request); err != nil {
			log.Println(err)
		}
	default:
		log.Fatal("Request type other than GET or Post not supported\n")
	}
}

func (*Server) get(writer http.ResponseWriter, request *http.Request){
	path := request.URL.Path
	if path == "/" {
		path = "./static/main.html"
	} else {
		path = "." + path
	}
	http.ServeFile(writer, request, path)
}

func (s *Server) post(writer http.ResponseWriter, request *http.Request) error {
	id := request.FormValue("message")
	o, err := s.getOutputOrder(id)
	outputOrderBytes, err := json.MarshalIndent(dataFactory.OutputOrder{
		OrderUID:        o.OrderUID,
		Entry:           o.Entry,
		TotalPrice:      o.TotalPrice,
		CustomerID:      o.CustomerID,
		TrackNumber:     o.TrackNumber,
		DeliveryService: o.DeliveryService,
	}, "", "\t")

	if err != nil {
		return err
	}

	if o.OrderUID != "" {
		_, err = fmt.Fprintf(writer, string(outputOrderBytes))
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) getOutputOrder(uid string) (*dataFactory.OutputOrder, error) {

	if val, ok := s.cache.Get(uid); ok {
		return val.(*dataFactory.OutputOrder), nil
	}

	res, err := s.db.Query(fmt.Sprintf(`select * from "Order" where OrderUID = '%s' limit 1;`, uid))
	if err != nil {
		log.Fatal(err)
	}

	order := dataFactory.Order{}
	var deliveryCost int
	var totalPrice int

	for res.Next() {
		if err = res.Scan(&order.OrderUID, &order.Entry, &order.InternalSignature,
			&order.Locale, &order.CustomerID, &order.TrackNumber, &order.DeliveryService,
			&order.Shardkey, &order.SmID, &order.PaymentID);
			err != nil {
			fmt.Println(err.Error())
			return nil, err
		}
	}

	res, err = s.db.Query(fmt.Sprintf(`select p.paymentid from "Payments" as p where paymentid = '%d';`,
		order.PaymentID))
	if err != nil {
		return nil, err
	}

	for res.Next() {
		res.Scan(&deliveryCost)
	}

	res, err = s.db.Query(fmt.Sprintf(`select i.chrtid, sum(i.totalprice) from "Items" as i where chrtid = '%s'
													group by i.chrtid;`, order.OrderUID))
	if err != nil {
		return nil, err
	}

	for res.Next() {
		res.Scan(&order.OrderUID, &totalPrice)
	}
	totalPrice += deliveryCost

	outputOrder := dataFactory.OutputOrder{
		OrderUID:        order.OrderUID,
		Entry:           order.Entry,
		TotalPrice:      totalPrice,
		CustomerID:      order.CustomerID,
		TrackNumber:     order.TrackNumber,
		DeliveryService: order.DeliveryService,
	}
	s.cache.Store(uid, &outputOrder)

	return &outputOrder, nil
}

func (s* Server) TryToRestore() (error) {

	res, err := s.db.Query(fmt.Sprintf(`select o.orderuid from "Order" as o`))

	var uid string

	for res.Next() {
		err = res.Scan(&uid)

		if err != nil {
			return err
		}

		outputOrder, err := s.getOutputOrder(uid)

		if err != nil {
			return err
		}

		s.cache.Restore(outputOrder)
	}

	if err != nil {
		return err
	}

	return nil
}