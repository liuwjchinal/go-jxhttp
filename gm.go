package main

import (
	"bytes"
	"crypto/des"
	"encoding/base64"
	"encoding/json"
	proto "github.com/golang/protobuf/proto"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

var deskey = []byte("be673d9dc7c5b4af2265fcb5")

func desEncode(b []byte) ([]byte, error) {
	td, err := des.NewTripleDESCipher(deskey)
	if err != nil {
		logger.Println(err)
		return nil, err
	}

	mod := len(b) % td.BlockSize()
	v := td.BlockSize() - mod

	for i := 0; i < v; i++ {
		b = append(b, byte(v))
	}

	n := len(b) / td.BlockSize()
	var rb []byte
	for i := 0; i < n; i++ {
		dst := make([]byte, td.BlockSize())
		td.Encrypt(dst, b[i*8:(i+1)*8])
		rb = append(rb, dst[:]...)
	}

	return rb, nil
}

func desDecode(b []byte) ([]byte, error) {
	td, err := des.NewTripleDESCipher(deskey)
	if err != nil {
		logger.Println(err)
		return nil, err
	}
	// blockMode := cipher.NewCBCDecrypter(block, key)
	// orig := make([]byte, len(b))
	// blockMode.CryptBlocks(orig, b)
	// logger.Println(string(orig))

	n := len(b) / td.BlockSize()
	var rb []byte
	for i := 0; i < n; i++ {
		dst := make([]byte, td.BlockSize())
		td.Decrypt(dst, b[i*8:(i+1)*8])
		rb = append(rb, dst[:]...)
	}

	lastValue := int(rb[len(rb)-1])
	logger.Println(string(rb[0 : len(rb)-lastValue]))

	// 移除最后的0
	return bytes.TrimRight(rb, string([]byte{0})), nil
}

type queryWhereJson struct {
	Server string `json:"server"`
}

type queryJson struct {
	Where  queryWhereJson `json:"where"`
	ZoneId string         `json:"zoneId"`
}

func GMDecode(b []byte) ([]byte, error) {
	d1, err := url.QueryUnescape(string(b))
	if err != nil {
		logger.Println(err)
		return nil, err
	}

	d2, err := base64.StdEncoding.DecodeString(d1)
	if err != nil {
		logger.Println(err)
		return nil, err
	}
	// logger.Println(d2)

	d3, err := desDecode(d2)
	if err != nil {
		logger.Println(err)
		return nil, err
	}

	return d3, nil
}

func GMEncode(b []byte) ([]byte, error) {
	d1, err := desEncode(b)
	if err != nil {
		logger.Println(err)
		return nil, err
	}

	d2 := base64.StdEncoding.EncodeToString(d1)
	if err != nil {
		logger.Println(err)
		return nil, err
	}

	d3 := url.QueryEscape(d2)
	if err != nil {
		logger.Println(err)
		return nil, err
	}

	return []byte(d3), nil
}

func getServerID(b []byte) []int {
	var q queryJson

	err := json.Unmarshal(b, &q)
	if err != nil {
		logger.Println(err)
		return nil
	}
	logger.Println(q)

	var serverid string

	if len(q.Where.Server) == 0 {
		serverid = q.ZoneId
	} else {
		serverid = q.Where.Server
	}

	r := strings.Split(serverid, ";")
	if len(r) < 2 {
		return nil
	}

	var a []int
	r2 := strings.Split(r[1], ",")
	for i := range r2 {
		v, err := strconv.Atoi(r2[i])
		if err != nil {
			continue
		}
		a = append(a, v)
	}
	return a
}

type gmResp struct {
	Code int                        `json:"code"`
	Msg  map[string]json.RawMessage `json:"msg"`
}

func forwardGM2Client(s *Server, id int, pb proto.Message, resp *gmResp, wg *sync.WaitGroup, lock *sync.Mutex) {
	c := s.FindClient(uint32(id))
	if c != nil {
		// wg.Add(1)
		s.NotifyClient(c, int(Command_CMD_GMOPERATE_REQ), pb, func(args *callbackArgs) {
			defer wg.Done()
			if args.err != nil {
				return
			}
			var rsp GMOperateRsp
			if err := proto.Unmarshal(args.body, &rsp); err != nil {
				logger.Println(err)
				return
			}
			logger.Println("gs resp:\n", rsp)
			lock.Lock()
			resp.Msg[strconv.Itoa(id)] = json.RawMessage(rsp.Json)
			lock.Unlock()
		})
	} else {
		wg.Done()
		logger.Println("cannot find gs:", id)
	}
}

func GMQuery(s *Server, b []byte) ([]byte, error) {
	d, err := GMDecode(b)
	if err != nil {
		logger.Println(err)
		return nil, err
	}
	logger.Println("GM Decode:\n", string(d))

	ids := getServerID(d)
	if err != nil {
		logger.Println(err)
		return nil, err
	}

	logger.Println("serverid:", ids)

	wg := new(sync.WaitGroup)

	var req GMOperateReq
	req.Json = d

	var rsp gmResp
	rsp.Msg = make(map[string]json.RawMessage)
	lock := new(sync.Mutex)

	for i := range ids {
		wg.Add(1)
		go forwardGM2Client(s, ids[i], &req, &rsp, wg, lock)
	}

	wg.Wait()

	b, err = json.Marshal(&rsp)
	if err != nil {
		logger.Println(err)
		return nil, err
	}

	b, err = GMEncode(d)
	if err != nil {
		logger.Println(err)
		return nil, err
	}

	return b, nil
}
