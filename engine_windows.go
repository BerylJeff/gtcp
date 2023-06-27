// Copyright (c) 2023 The Glibevent Authors. All rights reserved.
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
	"fmt"
	"glibevent/internal/gfd"
	"glibevent/internal/math"
	"glibevent/pkg/buffer/ring"
	"glibevent/pkg/errors"
	"log"
	"net"
	"runtime"
	"sync/atomic"
)

type Server struct {
	dec         deCoder
	svrh        handle
	network     string
	addr        string
	opts        *Options // options with engine
	ln          net.Listener
	connections connMatrix // loop connections storage
	cid         int32
}

func (s *Server) Server(protoAddr string, opts ...Option) error {
	if s.svrh == nil {
		log.Fatal("server handle nil")
		return nil
	}
	options := loadOptions(opts...)
	if options.LockOSThread && options.NumEventLoop > 10000 {
		log.Printf("too many event-loops under LockOSThread mode, should be less than 10,000 "+
			"while you are trying to set up %d\n", options.NumEventLoop)
		return errors.ErrTooManyEventLoopThreads
	}

	rbc := options.ReadBufferCap
	switch {
	case rbc <= 0:
		options.ReadBufferCap = MaxStreamBufferCap
	case rbc <= ring.DefaultBufferSize:
		options.ReadBufferCap = ring.DefaultBufferSize
	default:
		options.ReadBufferCap = math.CeilToPowerOfTwo(rbc)
	}
	wbc := options.WriteBufferCap
	switch {
	case wbc <= 0:
		options.WriteBufferCap = MaxStreamBufferCap
	case wbc <= ring.DefaultBufferSize:
		options.WriteBufferCap = ring.DefaultBufferSize
	default:
		options.WriteBufferCap = math.CeilToPowerOfTwo(wbc)
	}

	// Figure out the proper number of event-loops/goroutines to run.
	if options.Multicore {
		options.NumEventLoop = runtime.NumCPU()
	}
	if options.NumEventLoop > gfd.EventLoopIndexMax {
		options.NumEventLoop = gfd.EventLoopIndexMax
	}

	if options.NumEventLoop <= 0 {
		options.NumEventLoop = 1
	}
	network, addr := parseProtoAddr(protoAddr)
	s.network = network
	s.addr = addr
	s.opts = options
	s.Start()
	return nil
}

func (s *Server) process(c *conn) {
	defer c.Close()
	buf := make([]byte, 1024)
	for {
		n, err := c.Read(buf)
		if err != nil {
			s.svrh.OnClose(c)
			s.connections.delConn(int(c.sysfd))
			err = c.Close()
			c.inboundBuffer.Done()
			if err != nil {
				log.Printf("c.Close() failed due to error: %v", err)
			}
			return
		}
		if n > 0 {
			_, _ = c.inboundBuffer.Write(buf[:n])
		}

		if s.dec != nil {
			for {
				h := s.dec.NewHead()
				headData, bodyData, err := s.dec.Decode(h, c)
				if err != nil {
					break
				}
				s.svrh.OnTraffic(c, headData, bodyData)
			}
		} else {
			data, err := c.Peek(-1)
			if err != nil {
				log.Printf("c.Close() failed due to error: %v", err)
				break
			}
			s.svrh.OnReceive(c, data)
		}
	}
}

func (s *Server) Start() {
	ln, err := net.Listen(s.network, s.addr)
	if err != nil {
		fmt.Println("listen failed, err:", err)
		return
	}
	s.ln = ln
	defer ln.Close()
	for {
		c, err := ln.Accept()
		if err != nil {
			fmt.Println("accept failed, err:", err)
			continue
		}
		atomic.AddInt32(&s.cid, 1)
		tempConn := newTCPConn(c, atomic.LoadInt32(&s.cid))
		go s.process(tempConn)
	}
}
