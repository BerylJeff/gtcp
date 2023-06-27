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
	"bytes"
	"glibevent/pkg/buffer/elastic"
	"io"
	"net"
)

type conn struct {
	net.Conn
	sysfd         int32              // file descriptor
	inboundBuffer elastic.RingBuffer // buffer for leftover data from the peer
}

func newTCPConn(dstConn net.Conn, fd int32) (c *conn) {
	return &conn{
		Conn:          dstConn,
		sysfd:         fd,
		inboundBuffer: elastic.RingBuffer{},
	}
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

func (c *conn) resetBuffer() {
	c.inboundBuffer.Reset()
}
