package main

import (
	"flag"
	"log"
	"os"
	"time"
)

var logger *log.Logger

var listenAddr = flag.String("addr", ":20002", "listen address")

func init() {
	logger = log.New(os.Stderr, "", log.LstdFlags|log.Lshortfile)
}

func main() {
	flag.Parse()
	loadConfig()

	s := NewServer()
	go s.ListenAndServe(*listenAddr, ":8088")

	time.Sleep(2 * time.Second)

	max := 1
	for i := 0; i < max; i++ {
		go ClientTest()
	}

	for {
		time.Sleep(1000 * time.Second)
	}

	return
}
