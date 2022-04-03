package header

import (
	"bytes"
	"encoding/binary"
)

type IHeader interface {
	GetHeadLen() int
	UnPack(buff []byte) error
	GetMsgLen() int32
	GetMsgID() int32
	Pack()([]byte,error)
}

// 头定义需要指明字段长度， 不能 int , []byte, string ; exsample: int16 [10]byte
type HeadInfo struct {
	MsgLen   int32 //长度(包括当前包头的长度)
	MsgID    int32 //消息头
}

func (s *HeadInfo) GetHeadLen() int {
	return binary.Size(HeadInfo{})
}

func (s *HeadInfo) UnPack(buff []byte) error {
	reader := bytes.NewReader(buff)
	if err := binary.Read(reader, binary.BigEndian, s); err != nil {
		return err
	}
	return nil
}

func (s *HeadInfo) Pack()([]byte,error) {
	buff := &bytes.Buffer{}
	if err := binary.Write(buff, binary.BigEndian, s); err != nil {
		return nil, err
	}
	return buff.Bytes(), nil
}

func (s *HeadInfo) GetMsgLen() int32 {
	return s.MsgLen
}

func (s *HeadInfo) GetMsgID() int32{
	return s.MsgID
}


