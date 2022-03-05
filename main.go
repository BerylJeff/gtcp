package main

import (
	"gtcp/client"
	"gtcp/header"
	_ "gtcp/logger"
	"gtcp/server"
	"fmt"
	"log"
)

//type TagHeadInfo struct {
//	Tag      uint32 //标识
//	MajorVer uint8  //主版本号
//	MinorVer uint8  //修正版本号
//	MsgLen   uint32 //长度(包括当前包头的长度)
//	MsgID    uint16 //消息头
//	MsgSeq   uint16 //消息序列号，同一台计算机，同一次连接唯一性
//	PackCrc  uint16 //首先校验包体，16位CRC
//	HeadCrc  uint16 //最后校验包头，16位CRC
//}

type S5Client struct {
	client.MsgHandle
}

func (c *S5Client) OnReceive(header header.IHeader, body []byte) error {
	log.Printf("S5Client OnReceive  msgId:%d msglen:%d msg:%s", header.GetMsgID(), header.GetMsgLen(), string(body))

	return nil
}

//func (c *S5Client) SendMsg(MsgID uint16, data []byte) (int, error) {
//	head := &header.HeadInfo{
//		MsgLen: int32(len(data)),
//		MsgID:  MsgID,
//	}
//	buff, err := head.Pack()
//	if err != nil {
//		log.Println(err)
//		return 0, err
//	}
//	buff = append(buff, data...)
//
//	if n, err := c.Send(buff); err != nil {
//		log.Println("Send Buff Data error:, ", err, " Conn Writer exit")
//		return 0, err
//	} else {
//		return n, nil
//	}
//}

type S5Server struct {
	server.MsgHandle
}

func (s *S5Server) OnReceive(cid uint64, header header.IHeader, body []byte) error {
	log.Printf("S5Server OnReceive cid:%d msgId:%d msglen:%d msg:%s", cid, header.GetMsgID(), header.GetMsgLen(), string(body))
	s.SendMsg(cid, header.GetMsgID() + 100, body)
	return nil
}

func (s *S5Server) SendMsg(cid uint64, MsgID uint16, data []byte) (int, error) {
	head := &header.HeadInfo{
		MsgLen: int32(len(data)),
		MsgID:  MsgID,
	}
	buff, err := head.Pack()
	if err != nil {
		log.Println(err)
		return 0, err
	}
	buff = append(buff, data...)
	if n, err := s.Send(cid, buff); err != nil {
		log.Println("Send Buff Data error:, ", err, " Conn Writer exit")
		return 0, err
	} else {
		return n, nil
	}
}

func main() {

	go func() {
		msgHandle := &S5Server{}
		s := server.NewServer(msgHandle)
		s.Start("tcp", "127.0.0.1:9995")
	}()

	cli := client.NewClient(&S5Client{})
	err := cli.Open("tcp", "127.0.0.1:9995")
	if err != nil {
		log.Println(err)
		return
	}

	for i := 1; i < 5; i++ {
		data := []byte(fmt.Sprintf("client_:%d", i))
		head := &header.HeadInfo{
			MsgLen: int32(len(data)),
			MsgID:  uint16(i + 100),
		}
		buff, err := head.Pack()
		if err != nil {
			log.Println(err)
			continue
		}
		buff = append(buff, data...)
		_, err = cli.Send(buff)
		if err != nil {
			log.Println(err)
			return
		}
	}
	select {}
}
