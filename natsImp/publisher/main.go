package main

import (
	"encoding/json"
	"errors"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/stan.go"
	"goProj/config"
	"goProj/dataFactory"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {

	if len(os.Args) < 2 {
		return errors.New("Please indicate the path to the config file")
	}

	cfg := config.Get(os.Args[1])

	clusterUrls := strings.Join(cfg.ClusterUrls, ", ")
	sc, err := stan.Connect(cfg.ClusterId,
		"id1",
		stan.NatsURL(clusterUrls),
		stan.NatsOptions(
			nats.ReconnectWait(time.Second*4),
			nats.Timeout(time.Second*4)),
	)

	if err != nil {
		return err
	}

	ordersCreator := dataFactory.OrderCreator{}
	order := ordersCreator.Create(rand.Intn(10000))

	message, err := json.MarshalIndent(order, "", "\t")
	if err != nil {
		return err
	}

	err = sc.Publish(cfg.Subject, message)

	log.Printf("Published random %s\n", cfg.Subject)

	return nil
}