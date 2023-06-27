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

//go:build linux || freebsd || dragonfly || darwin
// +build linux freebsd dragonfly darwin

package glibevent

import (
	"fmt"
	"glibevent/internal/gfd"
	"glibevent/internal/math"
	"glibevent/internal/netpoll"
	"glibevent/internal/socket"
	"glibevent/pkg/buffer/ring"
	"glibevent/pkg/errors"
	"log"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

type Server struct {
	PollFd      int
	dec         deCoder
	svrh        handle
	network     string
	addr        string
	opts        *Options // options with engine
	inShutdown  int32    // whether the engine is in shutdown
	ln          *listener
	connections connMatrix // loop connections storage
}

func (eng *Server) isInShutdown() bool {
	return atomic.LoadInt32(&eng.inShutdown) == 1
}

// shutdown signals the engine to shut down.
func (eng *Server) shutdown(err error) {
	if err != nil && err != errors.ErrEngineShutdown {
		log.Printf("engine is being shutdown with error: %v", err)
	}
}

func (eng *Server) Server(protoAddr string, opts ...Option) error {
	if eng.svrh == nil {
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
	eng.network = network
	eng.addr = addr
	eng.opts = options
	return eng.Start()
}

func (eng *Server) Callback(fd int, ev netpoll.IOEvent) error {
	if fd != eng.ln.fd {
		if c := eng.connections.getConn(fd); c != nil {
			if ev&netpoll.OutEvents != 0 {
				return eng.eventWrite(c)
			}
			if ev&netpoll.InEvents != 0 {
				return eng.eventRead(c)
			}
			if c.sysfd != fd {
				log.Printf("Callback fd err :%d ev:%d", fd, c.sysfd)
			}

			return nil
		}
	} else {
		return eng.accept()
	}
	return nil
}

func (eng *Server) eventWrite(c *conn) (err error) {
	err = c.WriteFlush()
	if err != nil {
		eng.svrh.OnClose(c)
		eng.connections.delConn(c.sysfd)
		err = c.Close()
		if err != nil {
			log.Printf("c.Close() failed due to error: %v", err)
		}
		return nil
	}
	return nil
}

func (eng *Server) eventRead(c *conn) (err error) {
	err = c.eventRead()
	if err != nil {
		eng.svrh.OnClose(c)
		eng.connections.delConn(c.sysfd)
		err = c.Close()
		if err != nil {
			log.Printf("c.Close() failed due to error: %v", err)
		}
		return nil
	}
	if eng.dec != nil {
		for {
			h := eng.dec.NewHead()
			headData, bodydata, err := eng.dec.Decode(h, c)
			if err != nil {
				return nil
			}
			eng.svrh.OnTraffic(c, headData, bodydata)
		}
	} else {
		data, err := c.Peek(-1)
		if err != nil {
			log.Printf("c.Close() failed due to error: %v", err)
			return nil
		}
		eng.svrh.OnReceive(c, data)
	}

	return nil
}

func (eng *Server) accept() (err error) {
	nfd, sa, err := unix.Accept(eng.ln.fd)
	if err != nil {
		log.Printf("Accept() failed due to error: %v", err)
		err = unix.Close(nfd)
		if err != nil {
			log.Printf("c.Close() failed due to error: %v", err)
		}
		return err
	}
	err = unix.SetNonblock(nfd, true)
	if err != nil {
		log.Printf("fcntl nonblock: %v", err)
		return os.NewSyscallError("fcntl nonblock", err)
	}
	remoteAddr := socket.SockaddrToTCPOrUnixAddr(sa)
	conn := newTCPConn(eng.PollFd, nfd, remoteAddr, 1024)
	netpoll.AddRead(&conn.pollAttachment)
	out, action := eng.svrh.OnOpen(conn)
	if action == false {
		eng.svrh.OnClose(conn)
		err = conn.Close()
		if err != nil {
			log.Printf("c.Close() failed due to error: %v", err)
		}
		log.Printf("c.Close() connections quantity: %v", eng.connections.loadCount())
		return nil
	}
	eng.connections.addConn(conn.sysfd, conn)
	if len(out) > 0 {
		conn.Write(out)
	}

	return nil
}

func (eng *Server) Start() (err error) {
	eng.ln, err = initListener(eng.network, eng.addr, eng.opts)
	if err != nil {
		return err
	}
	defer eng.ln.close()
	eng.PollFd, err = netpoll.OpenPoller()
	if err != nil {
		return err
	}
	eng.connections.init()
	err = unix.SetNonblock(eng.ln.fd, true)
	if err != nil {
		return os.NewSyscallError("fcntl nonblock", err)
	}
	pollAttachment := netpoll.PollAttachment{PollFd: eng.PollFd, FD: eng.ln.fd}
	err = netpoll.AddRead(&pollAttachment)
	if err != nil {
		return err
	}
	if eng.opts.LockOSThread {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}
	// 定时任务
	go func() {
		eng.ticker()
	}()
	err = netpoll.Polling(eng.PollFd, eng.Callback)
	if err != nil {
		log.Println(err)
	}
	netpoll.ClosePoller(eng.PollFd)
	return fmt.Errorf("server stop")
}

func (eng *Server) ticker() {
	for {
		time.Sleep(time.Second * 30)
		fmt.Printf("当前连接数：%d\n", eng.connections.loadCount())
		eng.connections.iterate(func(c *conn) bool {
			return c.ConnInfo()
		})
	}
}
