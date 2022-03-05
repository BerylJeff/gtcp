package client

import (
	"gtcp/header"
	"log"
)

type IMsgParse interface {
	NewHeader() header.IHeader
	OnReceive(header header.IHeader, body []byte) error
	OnConnect() error
	OnCloseEvent()
	OnSend( sendN int)
}

type MsgHandle struct {
}

func (c *MsgHandle) NewHeader() header.IHeader {
	return &header.HeadInfo{}
}

func (c *MsgHandle) OnReceive(header header.IHeader, body []byte) error {
	log.Printf("MsgHandle OnReceive")
	return nil
}

func (c *MsgHandle) OnConnect() error {
	return nil
}
func (c *MsgHandle) OnCloseEvent() {
	log.Printf("MsgHandle OnCloseEvent")
}

func (c *MsgHandle) OnSend(sendN int) {}

