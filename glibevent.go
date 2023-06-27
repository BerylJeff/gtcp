// Copyright (c) 2019 The Glibevent Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package glibevent

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"time"
)

// MaxStreamBufferCap is the default buffer size for each stream-oriented connection(TCP/Unix).
var MaxStreamBufferCap = 64 * 1024 // 64KB

var (
	// shutdownPollInterval is how often we poll to check whether engine has been shut down during gnet.Stop().
	shutdownPollInterval = 500 * time.Millisecond
)

func parseProtoAddr(addr string) (network, address string) {
	network = "tcp"
	address = strings.ToLower(addr)
	if strings.Contains(address, "://") {
		pair := strings.Split(address, "://")
		network = pair[0]
		address = pair[1]
	}
	return
}

type handle interface {
	// OnBoot fires when the engine is ready for accepting connections.
	// The parameter engine has information and various utilities.
	OnBoot() bool
	// OnShutdown fires when the engine is being shut down, it is called right after
	// all event-loops and connections are closed.
	OnShutdown() bool
	// OnOpen fires when a new connection has been opened.
	// The parameter out is the return value which is going to be sent back to the peer.
	OnOpen(conn net.Conn) ([]byte, bool)
	// OnClose fires when a connection has been closed.
	// The parameter err is the last known connection error.
	OnClose(conn net.Conn) bool
	// OnTraffic fires when a local socket receives data from the peer.
	OnTraffic(conn net.Conn, head, data []byte) bool
	// OnReceive fires when a local socket receives data from the peer.
	OnReceive(conn net.Conn, data []byte) bool
	// OnTick fires immediately after the engine starts and will fire again
	OnTick() (delay time.Duration, _ bool)
}

type Header interface {
	GetLen() int
	Decode([]byte)
	Encode() []byte
	GetBodyLen() int
}

type deCoder interface {
	NewHead() Header
	Decode(head Header, reader *conn) ([]byte, []byte, error)
}

const maxHeaderLen int = 8 // bodyLen
type defaultHead struct {
	bodyLen int32
	msgId   int32
}

func (d *defaultHead) GetLen() int { return maxHeaderLen }

func (h *defaultHead) Decode(data []byte) {
	byteBuffer := bytes.NewBuffer(data)
	_ = binary.Read(byteBuffer, binary.LittleEndian, &h.bodyLen)
	_ = binary.Read(byteBuffer, binary.LittleEndian, &h.msgId)
}

func (h *defaultHead) Encode() []byte {
	result := make([]byte, 0)
	buffer := bytes.NewBuffer(result)
	binary.Write(buffer, binary.LittleEndian, h.bodyLen)
	binary.Write(buffer, binary.LittleEndian, h.msgId)
	return buffer.Bytes()
}

func (h *defaultHead) GetBodyLen() int { return int(h.bodyLen) }

type DefaultDecoder struct {
}

func (d *DefaultDecoder) Decode(head Header, reader *conn) ([]byte, []byte, error) {
	bufflen := reader.inboundBuffer.Buffered()
	headLen := head.GetLen()
	if bufflen < int(headLen) {
		return nil, nil, io.ErrShortBuffer
	}
	headData, err := reader.Peek(headLen)
	if err != nil {
		return nil, nil, err
	}
	head.Decode(headData)
	if bufflen < int(headLen+head.GetBodyLen()) {
		return nil, nil, io.ErrShortBuffer
	}
	ldisLen := 0
	discarded, err := reader.Discard(headLen)
	if err != nil {
		return nil, nil, err
	}
	ldisLen += discarded
	bodyData, err := reader.Peek(head.GetBodyLen())
	if err != nil {
		return nil, nil, err
	}
	discarded, err = reader.Discard(head.GetBodyLen())
	if err != nil {
		return nil, nil, err
	}
	ldisLen += discarded
	return headData, bodyData, err
}

func (d *DefaultDecoder) NewHead() Header {
	return &defaultHead{}
}

func NewServer(h handle, decode deCoder) *Server {
	return &Server{svrh: h, dec: decode}

}

func NewClient(h handle, decode deCoder) *Client {
	return &Client{svrh: h, dec: decode}
}
