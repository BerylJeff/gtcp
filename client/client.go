package client

import (
	"fmt"
	"io"
	"log"
	"net"
)

type Client struct {
	conn   *net.TCPConn
	msgHandler IMsgParse
}

func NewClient(msgHandle IMsgParse) *Client{
	if msgHandle == nil{
		msgHandle = &MsgHandle{}
	}
	return &Client{msgHandler : msgHandle}
}

func (c *Client)Open(network,addr string) error{
	tcpAddr, err := net.ResolveTCPAddr(network, addr)
	if err != nil {
		fmt.Printf("ResolveTCPAddr")
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err !=nil{
		log.Println(err)
		return  err
	}
	c.conn = conn
	go c.MsgHandle()
	return nil
}

func (c *Client) MsgHandle() {
	// 需要关闭 close
	defer func() {
		if err := c.conn.Close(); err != nil {
			log.Println("unpack error ", err)
		}
		c.msgHandler.OnCloseEvent()
	}()

	hd:= c.msgHandler.NewHeader()
	headData := make([]byte, hd.GetHeadLen())
	data := make([]byte, 0)
	for {
		//读取客户端的Msg head
		if n, err := io.ReadFull(c.conn, headData); err != nil {
			log.Printf("read msg head error:%v len:%d \n", err, n)
			break
		}
		//拆包
		err := hd.UnPack(headData)
		if err != nil {
			log.Printf("unpack error:%v ", err)
			break
		}
		dataLen := hd.GetMsgLen()
		if dataLen <= 0 {
			log.Println("dataLen <= 0")
			break
		}
		//根据 dataLen 读取 data，放在msg.Data中
		data = make([]byte, dataLen)
		if _, err := io.ReadFull(c.conn, data); err != nil {
			log.Println("read msg data error ", err)
			break
		}

		if err = c.msgHandler.OnReceive(hd, data);err != nil{
			log.Println("unpack error ", err)
			break
		}
	}
}


func (c *Client) Send(data []byte) (int, error) {
	if n, err := c.conn.Write(data); err != nil {
		log.Println("Send Buff Data error:, ", err, " Conn Writer exit")
		return n, err
	} else {
		c.msgHandler.OnSend(n)
		return n, nil
	}
}

func (c *Client) Disconnect() {
	err := c.conn.Close()
	if err != nil {
		log.Println(err)
		return
	}
	return
}