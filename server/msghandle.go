package server

import (
	"github.com/Jaswit/gtcp/header"
	"fmt"
	"log"
	"net"
	"sync"
)

type IMsgParse interface {
	setConnect(cid uint64, conn *net.TCPConn)
	getConnect(cid uint64) (*net.TCPConn, error)
	remove(cid uint64) (*net.TCPConn, error)
	removeAll()
	NewHeader() header.IHeader
	OnReceive(cid uint64, header header.IHeader, body []byte) error
	OnConnect(cid uint64) error
	OnCloseEvent(cid uint64)
	OnSend(cid uint64, sendN int)
	Disconnect(cid uint64)
}

type MsgHandle struct {
	connMap sync.Map
}

func (s *MsgHandle) NewHeader() header.IHeader {
	return &header.HeadInfo{}
}

func (s *MsgHandle) setConnect(cid uint64, conn *net.TCPConn) {
	s.connMap.Store(cid, conn)
}

func (s *MsgHandle) getConnect(cid uint64) (*net.TCPConn, error) {
	if v, ok := s.connMap.Load(cid); ok {
		return v.(*net.TCPConn), nil
	}
	return nil, fmt.Errorf("not find connect cid:%v", cid)
}

func (s *MsgHandle) remove(cid uint64) (*net.TCPConn, error) {
	if v, loaded := s.connMap.LoadAndDelete(cid); loaded {
		return v.(*net.TCPConn), nil
	}
	return nil, fmt.Errorf("not find connect cid:%v", cid)
}

func (s *MsgHandle) removeAll() {
	s.connMap.Range(func(key, value interface{}) bool {
		err := value.(*net.TCPConn).Close()
		if err != nil {
			log.Println(err)
			return true
		}
		s.connMap.Delete(key)
		return true
	})
}

// 从当前连接获取原始的socket TCPConn
func (s *MsgHandle) Send(cid uint64, data []byte) (int, error) {
	if v, ok := s.connMap.Load(cid); ok {
		if n, err := v.(*net.TCPConn).Write(data); err != nil {
			log.Println("Send Buff Data error:, ", err, " Conn Writer exit")
			return n, err
		} else {
			s.OnSend(cid, n)
			return n, nil
		}
	}
	return 0, fmt.Errorf("not find cid:%d \n", cid)
}

//停止连接，结束当前连接状态M
func (s *MsgHandle) Disconnect(cid uint64) {
	conn, err := s.getConnect(cid)
	if err != nil {
		log.Println(err)
		return
	}
	err = conn.Close()
	if err != nil {
		log.Println(err)
		return
	}
	return
}


func (s *MsgHandle) OnReceive(cid uint64, header header.IHeader, body []byte) error {
	return nil
}

func (s *MsgHandle) OnConnect(cid uint64) error {
	return nil
}
func (s *MsgHandle) OnCloseEvent(cid uint64) {
}

func (s *MsgHandle) OnSend(cid uint64, sendN int) {}
