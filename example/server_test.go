package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"glibevent"
	_ "glibevent/logger"
	"net"
	"testing"
	"time"
)

const (
	_cusHeaderLen int32 = 16
	_magicNumber  int32 = 40
	_version      int32 = 40
)

type MyHead struct {
	magicNum int32
	version  int32
	checkSum int32
	bodyLen  int32
}

func (d *MyHead) GetLen() int { return int(_cusHeaderLen) }

func (h *MyHead) Decode(data []byte) {
	byteBuffer := bytes.NewBuffer(data)
	_ = binary.Read(byteBuffer, binary.LittleEndian, &h.magicNum)
	_ = binary.Read(byteBuffer, binary.LittleEndian, &h.version)
	_ = binary.Read(byteBuffer, binary.LittleEndian, &h.bodyLen)
	_ = binary.Read(byteBuffer, binary.LittleEndian, &h.checkSum)
}

func (h *MyHead) Encode() []byte {
	result := make([]byte, 0)
	buffer := bytes.NewBuffer(result)
	binary.Write(buffer, binary.LittleEndian, h.magicNum)
	binary.Write(buffer, binary.LittleEndian, h.version)
	binary.Write(buffer, binary.LittleEndian, h.bodyLen)
	binary.Write(buffer, binary.LittleEndian, h.checkSum)
	return buffer.Bytes()
}

func (h *MyHead) GetBodyLen() int { return int(h.bodyLen) }

func (h *MyHead) SetHeadInfo(checkSum, bodyLen int32) {
	h.bodyLen = bodyLen
	h.checkSum = checkSum
	h.magicNum = _magicNumber
	h.version = _version
}

type MyDecoder struct {
	glibevent.DefaultDecoder
}

func (d *MyDecoder) NewHead() glibevent.Header {
	return &MyHead{}
}

type MyServerHandler struct {
}

func (s *MyServerHandler) OnOpen(conn net.Conn) ([]byte, bool) {
	return nil, true
}

func (s *MyServerHandler) OnBoot() bool {
	return true
}

// OnShutdown fires when the engine is being shut down, it is called right after
// all event-loops and connections are closed.
func (s *MyServerHandler) OnShutdown() bool {
	return true
}

// OnTick fires immediately after the engine starts and will fire again
// following the duration specified by the delay return value.
func (s *MyServerHandler) OnTick() (delay time.Duration, _ bool) {
	return 0, true
}

// OnTraffic fires when a local socket receives data from the peer and without decoder .
func (s *MyServerHandler) OnReceive(conn net.Conn, data []byte) bool {
	return true
}

func (s *MyServerHandler) OnClose(_ net.Conn) bool {
	//fmt.Println("ServerHandler OnClose")
	return true
}

func (s *MyServerHandler) OnTraffic(conn net.Conn, head, data []byte) bool {
	//fmt.Println("ServerHandler OnTraffic")
	h := MyHead{}
	h.Decode(head)
	//	fmt.Printf("MyServerHandler OnTraffic head:%+v data:%s\n", h, string(data))

	resData := append(head, data...)
	n, err := conn.Write(resData)
	if err != nil {
		fmt.Printf("decode msg failed, n:%d err:%v", n, err)
	}
	return true
}

const protoAddr = "tcp://127.0.0.1:10233"

func TestServer(t *testing.T) {
	svr := glibevent.NewServer(&MyServerHandler{}, &MyDecoder{})
	err := svr.Server(protoAddr)
	if err != nil {
		fmt.Printf("start server:%s fail err:%v", protoAddr, err)
		return
	}

}
