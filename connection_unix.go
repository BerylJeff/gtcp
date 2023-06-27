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
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"glibevent/internal/bs"

	"glibevent/internal/netpoll"
	"glibevent/internal/socket"
	"glibevent/pkg/buffer/elastic"
	gerrors "glibevent/pkg/errors"
	bsPool "glibevent/pkg/pool/byteslice"

	"golang.org/x/sys/unix"
)

type conn struct {
	sysfd          int                // file descriptor
	writeMtx       sync.Locker        // write lock
	inboundBuffer  elastic.RingBuffer // buffer for leftover data from the peer
	outboundBuffer elastic.Buffer     // buffer for data that is eligible to be sent to the peer
	laddr          net.Addr
	remoteAddr     net.Addr
	pollAttachment netpoll.PollAttachment
}

func newTCPConn(pollFd, fd int, remoteAddr net.Addr, writeBufferCap int) (c *conn) {
	//nfd := &netpoll.NetFD{Sysfd: fd}
	c = &conn{
		writeMtx:       &sync.Mutex{},
		sysfd:          fd,
		pollAttachment: netpoll.PollAttachment{FD: fd, PollFd: pollFd},
	}
	c.outboundBuffer.Reset(writeBufferCap)
	c.inboundBuffer.Reset()
	return
}

func (c *conn) ConnInfo() bool {
	if c.sysfd <= 0 {
		// 不存在 删除
		return false
	}
	ilen := c.inboundBuffer.Buffered()
	if ilen > 0 {
		data, err := c.Peek(-1)
		if err != nil {
			fmt.Printf("当前连接 fd:%d error:%s", c.sysfd, err.Error())
			return true
		}
		fmt.Printf("connection fd:%d  receive：%d 数据：%s\n", c.sysfd, ilen, string(data))
	}
	c.writeMtx.Lock()
	defer c.writeMtx.Unlock()
	oLen := c.outboundBuffer.Buffered()
	if oLen > 0 {
		buff := make([]byte, 0)
		data := c.outboundBuffer.Peek(-1)
		for i := 0; i < len(data); i++ {
			buff = append(buff, data[i]...)
		}
		fmt.Printf("当前连接 fd:%d  接收缓冲区数据量：%d 数据：%s\n", c.sysfd, oLen, string(buff))
	}
	return true

}

func (c *conn) release() {
	c.remoteAddr = nil
	if addr, ok := c.remoteAddr.(*net.TCPAddr); ok && len(addr.Zone) > 0 {
		bsPool.Put(bs.StringToBytes(addr.Zone))
	}
	c.inboundBuffer.Done()
	c.writeMtx.Lock()
	defer c.writeMtx.Unlock()
	c.outboundBuffer.Release()
	c.sysfd = -1
	c.laddr = nil
}

func (c *conn) Close() (err error) {
	log.Printf("(c *conn) Close()  fd=%d ", c.sysfd)

	c.writeMtx.Lock()
	// Send residual data in buffer back to the peer before actually closing the connection.
	if !c.outboundBuffer.IsEmpty() {
		for !c.outboundBuffer.IsEmpty() {
			iov := c.outboundBuffer.Peek(0)
			if len(iov) > iovMax {
				iov = iov[:iovMax]
			}
			if n, e := unix.Writev(c.sysfd, iov); e != nil {
				break
			} else { //nolint:revive
				_, _ = c.outboundBuffer.Discard(n)
			}
		}
	}
	c.writeMtx.Unlock()
	err = netpoll.Delete(&c.pollAttachment)
	if err != nil {
		log.Printf("failed to delete fd=%d from poller err: %v", c.sysfd, err)
	}

	err = unix.Close(c.sysfd)
	if err != nil {
		log.Printf("failed to delete fd=%d from poller err: %v", c.sysfd, err)
	}
	c.release()
	return err
}

func (c *conn) resetBuffer() {
	c.inboundBuffer.Reset()
}

// The same is true of socket implementations on many systems.
// See golang.org/issue/7812 and golang.org/issue/16266.
// Use 1GB instead of, say, 2GB-1, to keep subsequent reads aligned.
// ignoringEINTRIO is like ignoringEINTR, but just for IO calls.
func ignoringEINTRIO(fn func(fd int, p []byte) (int, error), fd int, p []byte) (int, error) {
	for {
		n, err := fn(fd, p)
		if err != syscall.EINTR {
			return n, err
		}
	}
}

const maxRW = 1 << 30

// Read implements io.Reader.
func (c *conn) eventRead() error {
	buffer := make([]byte, 1024)
	for {
		// n, err := unix.Read(c.sysfd, buffer)
		n, err := ignoringEINTRIO(unix.Read, c.sysfd, buffer)
		if n < 0 {
			if err == unix.EAGAIN {
				err = nil
			}
			return err
		} else if n == 0 {
			// log.Printf("(c *conn) eventRead close fd=%d n:%d err: %v", c.sysfd, n, err)
			err = unix.ECONNRESET
			return err
		} else {
			_, _ = c.inboundBuffer.Write(buffer[:n])
		}
	}
}

func (c *conn) Read(p []byte) (n int, err error) {
	return c.inboundBuffer.Read(p)
}

func (c *conn) Next(n int) (buf []byte, err error) {
	inBufferLen := c.inboundBuffer.Buffered()
	if n > inBufferLen {
		return nil, io.ErrShortBuffer
	} else if n <= 0 {
		n = inBufferLen
	}
	if c.inboundBuffer.IsEmpty() {
		return nil, io.ErrShortBuffer
	}
	head, tail := c.inboundBuffer.Peek(n)
	defer c.inboundBuffer.Discard(n) //nolint:errcheck
	if len(head) >= n {
		return head[:n], err
	}
	cache := bytes.Buffer{}
	cache.Reset()
	cache.Write(head)
	cache.Write(tail)
	return cache.Bytes(), err
}

func (c *conn) Peek(n int) (buf []byte, err error) {
	inBufferLen := c.inboundBuffer.Buffered()
	if n > inBufferLen {
		return nil, io.ErrShortBuffer
	} else if n <= 0 {
		n = inBufferLen
	}
	if c.inboundBuffer.IsEmpty() {
		return nil, io.ErrShortBuffer
	}
	head, tail := c.inboundBuffer.Peek(n)
	if len(head) >= n {
		return head[:n], err
	}
	cache := bytes.Buffer{}
	cache.Reset()
	cache.Write(head)
	cache.Write(tail)
	return cache.Bytes(), nil
}

func (c *conn) Discard(n int) (int, error) {
	inBufferLen := c.inboundBuffer.Buffered()
	if inBufferLen < n || n <= 0 {
		c.resetBuffer()
		return inBufferLen, nil
	}
	if c.inboundBuffer.IsEmpty() {
		return 0, io.ErrShortBuffer
	}

	discarded, _ := c.inboundBuffer.Discard(n)
	if discarded < inBufferLen {
		return discarded, nil
	}
	return n, nil
}

func (c *conn) Write(data []byte) (n int, err error) {
	n = len(data)
	c.writeMtx.Lock()
	defer c.writeMtx.Unlock()
	if !c.outboundBuffer.IsEmpty() {
		_, _ = c.outboundBuffer.Write(data)
		return
	}
	sent, err := unix.Write(c.sysfd, data)
	if sent < 0 {
		if err == unix.EAGAIN {
			_, _ = c.outboundBuffer.Write(data)
			err = netpoll.ModReadWrite(&c.pollAttachment)
		} else {
			c.Close()
		}
		return sent, err
	} else if sent == 0 {
		// log.Printf("(c *conn) Write close fd=%d  n:%d err: %v", c.sysfd, n, err)
		err = unix.ECONNRESET
		c.Close()
		return sent, err
	} else {
		if err != nil {
			c.Close()
			return -1, os.NewSyscallError("write", err)
		}
	}

	// Failed to send all data back to the peer, buffer the leftover data for the next round.
	if sent < n {
		_, _ = c.outboundBuffer.Write(data[sent:])
		err = netpoll.ModReadWrite(&c.pollAttachment)
	}

	return
}

func (c *conn) writev(bs [][]byte) (n int, err error) {
	for _, b := range bs {
		n += len(b)
	}
	c.writeMtx.Lock()
	defer c.writeMtx.Unlock()
	// If there is pending data in outbound buffer, the current data ought to be appended to the outbound buffer
	// for maintaining the sequence of network packets.
	if !c.outboundBuffer.IsEmpty() {
		_, _ = c.outboundBuffer.Writev(bs)
		return
	}
	sent, err := unix.Writev(c.sysfd, bs)
	// if err != nil {
	// 	log.Printf("(c *conn) writev fd=%d n:%d err: %d %+v", c.sysfd, n, err, err.Error())
	// }
	if sent < 0 {
		if err == unix.EAGAIN {
			_, _ = c.outboundBuffer.Writev(bs)
			err = netpoll.ModReadWrite(&c.pollAttachment)
		} else {
			c.Close()
		}
		return sent, err
	} else if sent == 0 {
		// log.Printf("(c *conn) writev close fd=%d n:%d err: %v", c.sysfd, n, err)
		err = unix.ECONNRESET
		c.Close()
		return sent, err
	} else {
		if err != nil {
			c.Close()
			return -1, os.NewSyscallError("write", err)
		}
	}

	// Failed to send all data back to the peer, buffer the leftover data for the next round.
	if sent < n {
		var pos int
		for i := range bs {
			bn := len(bs[i])
			if sent < bn {
				bs[i] = bs[i][sent:]
				pos = i
				break
			}
			sent -= bn
		}
		_, _ = c.outboundBuffer.Writev(bs[pos:])
		err = netpoll.ModReadWrite(&c.pollAttachment)
	}
	return
}

const iovMax = 1024

func (c *conn) WriteFlush() error {
	c.writeMtx.Lock()
	defer c.writeMtx.Unlock()
	iov := c.outboundBuffer.Peek(-1)
	var (
		n   int   = 0
		err error = nil
	)
	if len(iov) > 1 {
		if len(iov) > iovMax {
			iov = iov[:iovMax]
		}
		n, err = unix.Writev(c.sysfd, iov)
	} else {
		if len(iov) <= 0 {
			// 关闭描述符 调用
			n, err = unix.Write(c.sysfd, []byte{})
		} else {
			n, err = unix.Write(c.sysfd, iov[0])
		}
	}
	if n < 0 {
		if err == unix.EAGAIN {
			err = nil
		}
		return err
	} else if n == 0 {
		// log.Printf(" (c *conn) WriteFlush  fd=%d  n:%d ", c.sysfd, n)
		err = unix.ECONNRESET
		return err
	} else {
		_, _ = c.outboundBuffer.Discard(n)
	}

	if c.outboundBuffer.IsEmpty() {
		_ = netpoll.ModRead(&c.pollAttachment)
		return nil
	}
	return nil
}

func (c *conn) LocalAddr() net.Addr  { return c.laddr }
func (c *conn) RemoteAddr() net.Addr { return c.remoteAddr }

func (c *conn) Fd() int                              { return c.sysfd }
func (c *conn) Dup() (fd int, msg string, err error) { return netpoll.DupCloseOnExec(c.sysfd) }
func (c *conn) SetReadBuffer(bytes int) error        { return socket.SetRecvBuffer(c.sysfd, bytes) }
func (c *conn) SetWriteBuffer(bytes int) error       { return socket.SetSendBuffer(c.sysfd, bytes) }
func (c *conn) SetLinger(sec int) error              { return socket.SetLinger(c.sysfd, sec) }
func (c *conn) SetNoDelay(noDelay bool) error {
	return socket.SetNoDelay(c.sysfd, func(b bool) int {
		if b {
			return 1
		}
		return 0
	}(noDelay))
}

func (c *conn) SetKeepAlivePeriod(d time.Duration) error {
	return socket.SetKeepAlivePeriod(c.sysfd, int(d.Seconds()))
}

func (*conn) SetDeadline(_ time.Time) error {
	return gerrors.ErrUnsupportedOp
}

func (*conn) SetReadDeadline(_ time.Time) error {
	return gerrors.ErrUnsupportedOp
}

func (*conn) SetWriteDeadline(_ time.Time) error {
	return gerrors.ErrUnsupportedOp
}
