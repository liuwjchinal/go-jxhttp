package main

import (
	"log"
	"os"
	"time"
)

var logger *log.Logger

func init() {
	logger = log.New(os.Stderr, "", log.LstdFlags|log.Lshortfile)
}

func main() {
	loadConfig()

	s := NewServer()
	go s.ListenAndServe(":20002", ":8088")

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
