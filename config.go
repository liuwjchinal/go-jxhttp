package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
)

type RequestJsonElem struct {
	Cmd          int    `json:"cmd"`
	Url          string `json:"url"`
	RequestType  string `json:"request_type"`
	ResponseType string `json:"response_type"`
}

type NotifyJsonElem struct {
	Handle       string `json:"handle"`
	Cmd          int    `json:"cmd"`
	NotifyType   string `json:"notify_type"`
	ResponseType string `json:"response_type"`
}

var requestJsonMap map[int]*RequestJsonElem
var notifyJsonMap map[string]*NotifyJsonElem

func init() {
	requestJsonMap = make(map[int]*RequestJsonElem)
	notifyJsonMap = make(map[string]*NotifyJsonElem)
}

func loadGMServerJson() []byte {
	b, err := ioutil.ReadFile("gmserver.json")
	if err != nil {
		return nil
	}
	buf := bytes.NewBuffer(nil)
	err = json.Compact(buf, b)
	if err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}

func loadGMChannelJson() []byte {
	b, err := ioutil.ReadFile("gmchannel.json")
	if err != nil {
		return nil
	}
	buf := bytes.NewBuffer(nil)
	err = json.Compact(buf, b)
	if err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}

func loadRequestJson() error {
	b, err := ioutil.ReadFile("request.json")
	if err != nil {
		return err
	}
	var data []RequestJsonElem
	if err = json.Unmarshal(b, &data); err != nil {
		return err
	}
	logger.Println(data)
	for i := 0; i < len(data); i++ {
		requestJsonMap[data[i].Cmd] = &data[i]
	}
	return nil
}

func loadNotifyJson() error {
	b, err := ioutil.ReadFile("notify.json")
	if err != nil {
		return err
	}
	var data []NotifyJsonElem
	if err = json.Unmarshal(b, &data); err != nil {
		return err
	}
	logger.Println(data)
	for i := 0; i < len(data); i++ {
		notifyJsonMap[data[i].Handle] = &data[i]
	}
	return nil
}

func findRequestJsonElem(cmd int) *RequestJsonElem {
	if elem, ok := requestJsonMap[cmd]; ok {
		return elem
	}
	return nil
}

func findNotifyJsonElem(handle string) *NotifyJsonElem {
	if elem, ok := notifyJsonMap[handle]; ok {
		return elem
	}
	return nil
}

func loadConfig() error {
	err := loadRequestJson()
	if err != nil {
		logger.Fatal(err)
		return err
	}
	err = loadNotifyJson()
	if err != nil {
		logger.Fatal(err)
		return err
	}
	return nil
}
