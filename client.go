// test client

package main

import (
	"encoding/binary"
	proto "github.com/golang/protobuf/proto"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

func sendAndRecv(conn net.Conn, cmd, seq int, pb proto.Message, pb2 proto.Message) {
	b := make([]byte, 1024)
	body, err := proto.Marshal(pb)
	logger.Printf("send bytes %v\n", body)
	if err != nil {
		logger.Fatal(err)
	}

	// cmd
	binary.LittleEndian.PutUint16(b[0:2], uint16(cmd))
	// len
	binary.LittleEndian.PutUint16(b[2:4], uint16(8+len(body)))
	// seq
	binary.LittleEndian.PutUint32(b[4:8], 1)

	conn.Write(b[0:8])
	conn.Write(body)

	if n, err := io.ReadFull(conn, b[0:8]); n < HeaderSize || err != nil {
		logger.Fatal(err)
	}

	size := int(binary.LittleEndian.Uint16(b[2:4]) - 8)
	if size == 0 {
		return
	}
	if n, err := io.ReadFull(conn, b[0:size]); n < size || err != nil {
		logger.Fatal(err)
	}

	if pb2 != nil {
		err := proto.Unmarshal(b[0:size], pb2)
		if err != nil {
			logger.Fatal(err)
		}
		logger.Println("recv resp:", pb2)
	}
}

func sendRegister(conn net.Conn) {
	var req RegisterReq
	req.Id = proto.Uint32(1000)
	sendAndRecv(conn, int(Command_CMD_REGISTER_REQ), 1, &req, nil)
}

func sendVerify(conn net.Conn) {
	var req VerifySessionReq
	var rsp VerifySessionResp
	req.AuthInfo = proto.String("xxooxxoo")
	sendAndRecv(conn, int(Command_CMD_VERIFYSESSION_REQ), 1, &req, &rsp)
}

func sendPayNotify() {
	resp, err := http.Get("http://127.0.0.1:8088/notify?fn=paynotify&appGoodsAmount=1&appGoodsDesc=paymentDes017&appGoodsId=payment019&appGoodsName=10&appId=xyfm&channelId=xiaomi&custom=1000&orderId=100265&originalPrice=1&payStatus=1&payTime=&sdkUid=25613430&totalPrice=1&sign=fc1d381e6f45f92caadd1d923a1beb6e")
	if err != nil {
		logger.Println(err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	logger.Println(string(body))
}

func ClientTest() {
	conn, err := net.Dial("tcp", ":20002")
	if err != nil {
		logger.Fatal(err)
	}

	sendRegister(conn)
	time.Sleep(1 * time.Second)
	sendVerify(conn)
	time.Sleep(1 * time.Second)
	sendPayNotify()
}
