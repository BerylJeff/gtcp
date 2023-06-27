package main

import (
	"fmt"
	"glibevent"
	"net"
	"strconv"
	"testing"
	"time"
)

type MyClientHandler struct {
}

func (s *MyClientHandler) OnOpen(conn net.Conn) ([]byte, bool) {
	return nil, true
}

func (s *MyClientHandler) OnBoot() bool {
	return true
}

// OnShutdown fires when the engine is being shut down, it is called right after
// all event-loops and connections are closed.
func (s *MyClientHandler) OnShutdown() bool {
	return true
}

// OnTick fires immediately after the engine starts and will fire again
// following the duration specified by the delay return value.
func (s *MyClientHandler) OnTick() (delay time.Duration, _ bool) {
	return 0, true
}

// OnTraffic fires when a local socket receives data from the peer and without decoder .
func (s *MyClientHandler) OnReceive(conn net.Conn, data []byte) bool {
	return true
}

func (s *MyClientHandler) OnClose(_ net.Conn) bool {
	//fmt.Println("ServerHandler OnClose")
	return true
}

func (s *MyClientHandler) OnTraffic(conn net.Conn, head, data []byte) bool {
	//fmt.Println("ServerHandler OnTraffic")
	h := MyHead{}
	h.Decode(head)
	fmt.Printf("MyClientHandler OnTraffic head:%+v data:%s\n", h, string(data))

	return true
}

func ClientTest(num int) {
	cli := glibevent.NewClient(&MyClientHandler{}, &MyDecoder{})
	err := cli.Open(protoAddr)
	if err != nil {
		fmt.Printf("decode msg failed err:%v", err)
		return
	}
	data := strconv.Itoa(num) + "-hello word-"
	for i := 0; i < 100; i++ {
		temp := data + strconv.Itoa(i)
		head := &MyHead{}
		head.SetHeadInfo(int32(i), int32(len(temp)))
		head.magicNum = int32(num)
		sendbuf := head.Encode()
		sendbuf = append(sendbuf, []byte(temp)...)
		n, err := cli.C.Write(sendbuf)
		if err != nil {
			fmt.Printf("decode msg failed, n:%d err:%v", n, err)
			return
		}
	}
}

func TestClient(t *testing.T) {
	ClientTest(0)
	select {}
}
