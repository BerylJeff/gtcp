package glibevent

import (
	"bytes"
	"fmt"
	"glibevent/pkg/buffer/elastic"
	"io"
	"log"
	"net"
)

type Client struct {
	C             net.Conn
	dec           deCoder
	svrh          handle
	inboundBuffer elastic.RingBuffer // buffer for leftover data from the peer
}

func (cli *Client) Peek(n int) (buf []byte, err error) {
	inBufferLen := cli.inboundBuffer.Buffered()
	if n > inBufferLen {
		return nil, io.ErrShortBuffer
	} else if n <= 0 {
		n = inBufferLen
	}
	if cli.inboundBuffer.IsEmpty() {
		return nil, io.ErrShortBuffer
	}
	head, tail := cli.inboundBuffer.Peek(n)
	if len(head) >= n {
		return head[:n], err
	}
	cache := bytes.Buffer{}
	cache.Reset()
	cache.Write(head)
	cache.Write(tail)
	return cache.Bytes(), nil
}

func (cli *Client) Discard(n int) (int, error) {
	inBufferLen := cli.inboundBuffer.Buffered()
	if inBufferLen < n || n <= 0 {
		cli.resetBuffer()
		return inBufferLen, nil
	}
	if cli.inboundBuffer.IsEmpty() {
		return 0, io.ErrShortBuffer
	}

	discarded, _ := cli.inboundBuffer.Discard(n)
	if discarded < inBufferLen {
		return discarded, nil
	}
	return n, nil
}

func (cli *Client) resetBuffer() {
	cli.inboundBuffer.Reset()
}
func (cli *Client) Decode(head Header) ([]byte, []byte, error) {
	bufflen := cli.inboundBuffer.Buffered()
	headLen := head.GetLen()
	if bufflen < int(headLen) {
		return nil, nil, io.ErrShortBuffer
	}
	headData, err := cli.Peek(headLen)
	if err != nil {
		return nil, nil, err
	}
	head.Decode(headData)
	if bufflen < int(headLen+head.GetBodyLen()) {
		return nil, nil, io.ErrShortBuffer
	}
	ldisLen := 0
	discarded, err := cli.Discard(headLen)
	if err != nil {
		return nil, nil, err
	}
	ldisLen += discarded
	bodyData, err := cli.Peek(head.GetBodyLen())
	if err != nil {
		return nil, nil, err
	}
	discarded, err = cli.Discard(head.GetBodyLen())
	if err != nil {
		return nil, nil, err
	}
	ldisLen += discarded
	return headData, bodyData, err
}

func (cli *Client) process(c net.Conn) {
	defer c.Close()
	buf := make([]byte, 1024)
	for {
		n, err := c.Read(buf)
		if err != nil {
			cli.svrh.OnClose(c)
			err = c.Close()
			cli.inboundBuffer.Done()
			if err != nil {
				log.Printf("c.Close() failed due to error: %v", err)
			}
			return
		}
		if n > 0 {
			_, _ = cli.inboundBuffer.Write(buf[:n])
		}

		if cli.dec != nil {
			for {
				h := cli.dec.NewHead()
				headData, bodyData, err := cli.Decode(h)
				if err != nil {
					break
				}
				cli.svrh.OnTraffic(c, headData, bodyData)
			}
		} else {
			data, err := cli.Peek(-1)
			if err != nil {
				log.Printf("c.Close() failed due to error: %v", err)
				break
			}
			cli.svrh.OnReceive(c, data)
		}
	}
}

func (cli *Client) Open(protoAddr string) error {
	network, addr := parseProtoAddr(protoAddr)
	c, err := net.Dial(network, addr)
	if err != nil {
		fmt.Println("listen failed, err:", err)
		return err
	}
	cli.svrh.OnOpen(c)
	cli.C = c
	go cli.process(c)
	return nil
}
