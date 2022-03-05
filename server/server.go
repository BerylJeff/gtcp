package server

import (
	"io"
	"log"
	"net"
)

type Server struct {
	msgHandler IMsgParse
	listener *net.TCPListener
}

func NewServer(msgHandle IMsgParse) *Server{
	if msgHandle == nil{
		msgHandle = &MsgHandle{}
	}
	return &Server{msgHandler : msgHandle}
}

func (s *Server) Start(network, addr string) {
	//1 获取一个TCP的Addr
	addrInfo, err := net.ResolveTCPAddr(network, addr)
	if err != nil {
		log.Println("resolve tcp addr err: ", err)
		return
	}
	//2 监听服务器地址
	s.listener, err = net.ListenTCP(network, addrInfo)
	if err != nil {
		log.Println("listen", network, "err", err)
		return
	}
	//TODO server.go 应该有一个自动生成ID的方法
	var cid uint64 = 0
	//3 启动server网络连接业务
	for {
		//3.1 阻塞等待客户端建立连接请求
		conn, err := s.listener.AcceptTCP()
		if err != nil {
			log.Println("Accept err ", err)
			continue
		}
		cid++
		if err = s.msgHandler.OnConnect(cid);err !=nil{
			err = conn.Close()
			if err != nil {
				log.Println("Accept err ", err)
				continue
			}
			s.msgHandler.OnCloseEvent(cid)
			continue
		}

		s.msgHandler.setConnect(cid, conn)
		//3.4 启动当前链接的处理业务
		go s.MsgHandle(cid, conn)
	}
	s.listener.Close()
	s.msgHandler.removeAll()
}

/*
	读消息Goroutine，用于从客户端中读取数据
*/
func (s *Server) MsgHandle(cid uint64, conn *net.TCPConn) {
	// 需要关闭 close
	defer func() {
		if err := conn.Close(); err != nil {
			log.Println("unpack error ", err)
		}
		s.msgHandler.remove(cid)
		s.msgHandler.OnCloseEvent(cid)
	}()

	hd:= s.msgHandler.NewHeader()
	headData := make([]byte, hd.GetHeadLen())
	data := make([]byte, 0)
	for {
		//读取客户端的Msg head
		if n, err := io.ReadFull(conn, headData); err != nil {
			log.Printf("read msg head error:%v len:%d \n", err, n)
			break
		}
		//拆包，得到msgid 和 datalen 放在msg中
		err := hd.UnPack(headData)
		if err != nil {
			log.Printf("unpack error:%v ", err)
			break
		}

		dataLen := hd.GetMsgLen()
		if dataLen <= 0 {
			log.Println("msg len <= 0")
			break
		}
		//根据 dataLen 读取 data，放在msg.Data中
		data = make([]byte, dataLen)
		if _, err := io.ReadFull(conn, data); err != nil {
			log.Println("read msg data error ", err)
			break
		}
		if err = s.msgHandler.OnReceive(cid, hd, data);err != nil{
			log.Println("unpack error ", err)
			break
		}
	}
}



