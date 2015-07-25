package main

import (
	proto "github.com/golang/protobuf/proto"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"sync"
)

func XGSDKReadRequest(s *Server, c *Client, cmd int, seq uint32, b []byte) error {
	var err error

	cfg := findRequestJsonElem(cmd)
	if cfg == nil {
		return ErrUnknownType
	}
	// logger.Println(cfg)

	typ := findtyp(cfg.RequestType)
	if typ == nil {
		return ErrUnknownType
	}

	// new val is a pointer that point to the pb msg
	val := reflect.New(typ)
	// get the real pointer
	ptr := val.Interface()
	if err = proto.Unmarshal(b, ptr.(proto.Message)); err != nil {
		logger.Println(err)
		return err
	}

	logger.Println("recv:", ptr.(proto.Message))

	// async
	go XGSDKWriteResponse(s, c, cmd, seq, ptr.(proto.Message))

	return nil
}

func XGSDKWriteResponse(s *Server, c *Client, cmd int, seq uint32, pb proto.Message) error {
	cfg := findRequestJsonElem(cmd)
	if cfg == nil {
		return ErrUnknownType
	}

	getreq, err := pbtohttpget(pb)
	if err != nil {
		logger.Println(err)
		return err
	}

	// send http request and read response
	result, err := http.Get(cfg.Url + string(getreq))
	if err != nil {
		return err
	}
	defer result.Body.Close()

	body, err := ioutil.ReadAll(result.Body)
	if err != nil {
		return err
	}
	logger.Println(string(body))

	resp, err := jsontopb(body, cfg.ResponseType)
	if err != nil {
		logger.Println(err)
		return err
	}
	logger.Println(resp)

	return s.writeResponse(c, cmd, seq, resp)
}

func XGSDKReadNotify(s *Server, fn string, w http.ResponseWriter, r *http.Request) error {
	logger.Println("XGSDKReadNotify")
	cfg := findNotifyJsonElem(fn)
	if cfg == nil {
		logger.Printf("unknown fn %s\n", fn)
		return ErrUnknownType
	}

	typ := findtyp(cfg.NotifyType)
	if typ == nil {
		logger.Printf("unknown type %s\n", cfg.NotifyType)
		return ErrUnknownType
	}

	custom := r.FormValue("custom")
	if len(custom) == 0 {
		logger.Printf("no gsid\n")
		return ErrUnknownGSID
	}

	gsid, err := strconv.Atoi(custom)
	if err != nil {
		logger.Println(err)
		return ErrUnknownGSID
	}

	c := s.FindClient(uint32(gsid))
	if c == nil {
		logger.Printf("unknown gs id %d\n", gsid)
		return ErrUnknownGSID
	}

	// new val is a pointer that point to the pb msg
	val := reflect.New(typ)
	// get the real pointer
	ptr := val.Interface()

	err = httpgettopb(r, ptr)
	if err != nil {
		logger.Println(err)
		return err
	}

	logger.Println(ptr.(proto.Message))

	wait := new(sync.Mutex)
	wait.Lock()
	s.NotifyClient(c, cfg.Cmd, ptr.(proto.Message), func(args *callbackArgs) {
		defer wait.Unlock()
		// resp, err := pbtojson(pb)
		if err != nil {
			logger.Println(err)
			return
		}
		// w.Write(resp)
	})

	// wait gs response
	wait.Lock()
	return nil
}
