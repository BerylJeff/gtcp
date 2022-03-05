package server

import (
	"commonTcpServer/header"
	"log"
	"testing"
)
type S5Server struct {
	MsgHandle
}

func (s *S5Server) OnReceive(cid uint64, header header.IHeader, body []byte) error {
	log.Printf("my OnReceive cid:%d msgId:%d msglen:%d msg:%s", cid, header.GetMsgID(), header.GetMsgID(), string(body))
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

func TestServer(T *testing.T) {
	msgHandle := &S5Server{}
	s := NewServer(msgHandle)
	s.Start("tcp", "127.0.0.1:9995")
}