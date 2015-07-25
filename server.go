package main

import (
	"container/list"
	"encoding/binary"
	"errors"
	proto "github.com/golang/protobuf/proto"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"time"
)

var ErrInvalidPacket = errors.New("invalid packet")
var ErrInvalidCmd = errors.New("invalid command")
var ErrUnknownGSID = errors.New("unknown gsid")
var HeaderSize = 8

type Header struct {
	Cmd uint16
	Len uint16
	Seq uint32
}

type Server struct {
	ntfch      chan []byte
	clientlock sync.RWMutex
	clientmap  map[uint32]*Client
}

type pendingRequest struct {
	seq      uint32
	cb       NotifyCallback
	deadline time.Time
}

type Client struct {
	id       uint32   // GS ID
	conn     net.Conn // 当前GS连接
	r        io.Reader
	w        io.Writer
	rb       []byte     // 接收buffer
	sendlock sync.Mutex // protect wb
	wb       []byte     // 发送buffer
	seqlock  sync.Mutex // protect seq and seqmap
	seq      uint32
	seqmap   map[uint32]*pendingRequest
	timelist *list.List
}

func (c *Client) addPendingRequest(cb NotifyCallback) uint32 {
	r := &pendingRequest{
		cb: cb,
	}
	c.seqlock.Lock()
	r.seq = c.seq
	c.seq++
	c.seqmap[r.seq] = r
	c.seqlock.Unlock()
	return r.seq
}

func newClient(conn net.Conn) *Client {
	c := &Client{
		id:     0,
		conn:   conn,
		r:      conn,
		w:      conn,
		rb:     make([]byte, 1024),
		wb:     make([]byte, 1024),
		seqmap: make(map[uint32]*pendingRequest),
	}
	return c
}

type callbackArgs struct {
	s    *Server
	c    *Client
	cmd  int
	seq  uint32
	body []byte
	err  error
}

type ClientReader func(*Server, *Client, int, uint32, []byte) error
type NotifyReader func(*Server, string, http.ResponseWriter, *http.Request) error
type NotifyCallback func(*callbackArgs)

func (s *Server) readRequestHeader(c *Client) (cmd int, size int, seq uint32, err error) {
	if n, err := io.ReadFull(c.r, c.rb[0:HeaderSize]); n < HeaderSize || err != nil {
		logger.Println(err)
		return 0, 0, 0, err
	}

	cmd = int(binary.LittleEndian.Uint16(c.rb[0:2]))
	size = int(binary.LittleEndian.Uint16(c.rb[2:4]))
	seq = binary.LittleEndian.Uint32(c.rb[4:8])
	return cmd, size, seq, nil
}

func (s *Server) NotifyClient(c *Client, cmd int, pb proto.Message, cb NotifyCallback) error {
	var err error

	seq := c.addPendingRequest(cb)

	c.sendlock.Lock()
	defer c.sendlock.Unlock()

	// cmd
	binary.LittleEndian.PutUint16(c.wb[0:2], uint16(cmd))
	// seq
	binary.LittleEndian.PutUint32(c.wb[4:8], seq)
	// pb
	if pb != nil {
		buf := proto.NewBuffer(c.wb[0:8])
		buf.Marshal(pb)
		respb := buf.Bytes()
		// len
		binary.LittleEndian.PutUint16(respb[2:4], uint16(len(respb)))
		_, err = c.w.Write(respb)
	} else {
		// len
		binary.LittleEndian.PutUint16(c.wb[2:4], uint16(HeaderSize))
		_, err = c.w.Write(c.wb[0:8])
	}

	return err
}

func (s *Server) writeResponse(c *Client, cmd int, seq uint32, pb proto.Message) error {
	var err error

	c.sendlock.Lock()
	defer c.sendlock.Unlock()

	// cmd
	binary.LittleEndian.PutUint16(c.wb[0:2], uint16(cmd+1))
	// seq
	binary.LittleEndian.PutUint32(c.wb[4:8], seq)
	logger.Println("send header:", cmd+1, seq)
	// pb
	if pb != nil {
		buf := proto.NewBuffer(c.wb[0:8])
		buf.Marshal(pb)
		respb := buf.Bytes()
		// len
		binary.LittleEndian.PutUint16(respb[2:4], uint16(len(respb)))
		_, err = c.w.Write(respb)
	} else {
		// len
		binary.LittleEndian.PutUint16(c.wb[2:4], uint16(HeaderSize))
		_, err = c.w.Write(c.wb[0:8])
	}
	return err
}

func (s *Server) readRequest(c *Client) error {
	cmd, size, seq, err := s.readRequestHeader(c)
	if err != nil {
		return err
	}

	logger.Println("recv header:", cmd, size, seq)

	bodysize := size - HeaderSize
	if bodysize <= 0 {
		logger.Println("invalid body size:", bodysize)
		return ErrInvalidPacket
	}

	// need grow?
	if len(c.rb) < bodysize {
		c.rb = make([]byte, bodysize)
	}

	if n, err := io.ReadFull(c.r, c.rb[0:bodysize]); n < bodysize || err != nil {
		logger.Println(err, bodysize)
		return ErrInvalidPacket
	}

	reader := s.findProc(Command(cmd))
	if reader == nil {
		logger.Println("invalid cmd:", cmd)
		return ErrInvalidCmd
	}
	return reader(s, c, cmd, seq, c.rb[0:bodysize])
}

func (s *Server) Notitfy(b []byte) error {
	s.ntfch <- b
	return nil
}

func procRegister(s *Server, c *Client, cmd int, seq uint32, body []byte) error {
	var req RegisterReq
	if err := proto.Unmarshal(body, &req); err != nil {
		logger.Println(err, body)
		return err
	}

	c.id = req.GetId()

	s.clientlock.Lock()
	s.clientmap[c.id] = c
	s.clientlock.Unlock()

	logger.Println("New GS registerd:", c.id)

	s.writeResponse(c, cmd, seq, nil)
	return nil
}

func recvNotifyResp(s *Server, c *Client, cmd int, seq uint32, body []byte) error {
	c.seqlock.Lock()
	pending, ok := c.seqmap[seq]
	if ok {
		delete(c.seqmap, seq)
	}
	c.seqlock.Unlock()
	if !ok {
		return nil
	}
	args := &callbackArgs{
		cmd:  cmd,
		body: body,
		seq:  seq,
		s:    s,
		c:    c,
	}

	pending.cb(args)
	return nil
}

func (s *Server) FindClient(id uint32) *Client {
	s.clientlock.RLock()
	defer s.clientlock.RUnlock()

	if c, ok := s.clientmap[id]; ok {
		return c
	}
	return nil
}

func (s *Server) findProc(cmd Command) ClientReader {
	switch cmd {
	case Command_CMD_REGISTER_REQ:
		return procRegister
	case Command_CMD_VERIFYSESSION_REQ, Command_CMD_VERIFYORDER_REQ:
		return XGSDKReadRequest
	default:
		return recvNotifyResp
	}
	return nil
}

func (s *Server) findNotifyProc(fn string) NotifyReader {
	switch fn {
	case "paynotify":
		return XGSDKReadNotify
	}
	return nil
}

func (s *Server) findCallback(seq uint32) NotifyCallback {
	return nil
}

func (s *Server) removeClient(c *Client) {
	if c.id != 0 {
		s.clientlock.Lock()
		delete(s.clientmap, c.id)
		s.clientlock.Unlock()
	}
	c.conn.Close()
}

func (s *Server) serveConn(conn net.Conn) {
	var err error
	c := newClient(conn)

	defer s.removeClient(c)

	for {
		err = s.readRequest(c)
		if err != nil {
			break
		}
	}
}

func (s *Server) ListenAndServe(addr1, addr2 string) error {
	// start httpd
	s.initHttpServer()
	go http.ListenAndServe(addr2, nil)

	// start tcp server
	l, err := net.Listen("tcp", addr1)
	if err != nil {
		logger.Fatal(err)
		return err
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}

		// log.Printf("new client 1\n")
		go s.serveConn(conn)
		// log.Printf("new client 2\n")
	}
	return nil
}

func (s *Server) initHttpServer() error {
	http.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) {
		logger.Println(r)
		fn := r.FormValue("fn")
		logger.Println(fn)

		proc := s.findNotifyProc(fn)
		if proc == nil {
			logger.Printf("unknown fn %s\n", fn)
			return
		}
		proc(s, fn, w, r)
	})

	http.HandleFunc("/gm", func(w http.ResponseWriter, r *http.Request) {
		logger.Println(r)
		b, _ := ioutil.ReadAll(r.Body)
		b, err := GMQuery(s, b)
		if err != nil {
			logger.Println(err)
			return
		}
		w.Write(b)
	})

	http.HandleFunc("/gminfo", func(w http.ResponseWriter, r *http.Request) {
		logger.Println(r)
		fn := r.FormValue("fn")
		var b []byte
		if fn == "zone" {
			b = loadGMServerJson()
		} else if fn == "channel" {
			b = loadGMChannelJson()
		} else {
			logger.Println("invalid gminfo fn:", fn)
			return
		}
		logger.Println("resp:\n", string(b))
		d, err := GMEncode(b)
		if err != nil {
			logger.Println(err)
		}
		w.Write(d)

		d, err = GMDecode(d)
		logger.Println(string(d))
	})
	return nil
}

func NewServer() *Server {
	s := &Server{
		ntfch:     make(chan []byte, 1024),
		clientmap: make(map[uint32]*Client),
	}

	return s
}
