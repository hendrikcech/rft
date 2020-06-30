package rftp

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"sync"
)

type Lister interface {
	List() ([]os.FileInfo, error)
}

type Server struct {
	SRC     Lister
	connMgr *connManager
}

func NewServer(l Lister) *Server {
	return &Server{
		SRC: l,
		connMgr: &connManager{
			conns: make(map[string]*connection),
		},
	}
}

func (s *Server) Listen(host string) error {
	addr, err := net.ResolveUDPAddr("udp", host)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	defer conn.Close()

	for {
		msg := make([]byte, 1024)
		n, addr, err := conn.ReadFromUDP(msg)
		if err != nil {
			return err
		}

		go s.handlePacket(conn, addr, n, msg)
	}
}

func (s *Server) handlePacket(conn *net.UDPConn, addr *net.UDPAddr, length int, packet []byte) {
	header := &MsgHeader{}
	if err := header.UnmarshalBinary(packet); err != nil {
	    // TODO: Drop connection
	}

	switch header.msgType {
	case msgClientRequest:
		cr := &ClientRequest{}
		err := cr.UnmarshalBinary(packet[header.hdrLen:])
		if err != nil {
			// TODO: Drop connection
		}
		s.connMgr.accept(conn, addr, cr, s.SRC)

	case msgClientAck:
		ack := &Acknowledgement{}
		ack.UnmarshalBinary(packet[header.hdrLen:])
		s.connMgr.handle(addr, ack)

	case msgClose:
		s.connMgr.close(addr)

	default:
		// TODO: Drop connection
	}
}

type connection struct {
	ch   chan *Acknowledgement
	sock io.Writer
}

type connManager struct {
	mux   sync.Mutex
	conns map[string]*connection
}

func key(ip *net.UDPAddr) string {
	return fmt.Sprintf("%v:%v", ip.IP, ip.Port)
}

type responseWriter func([]byte) (int, error)

func (rw responseWriter) Write(bs []byte) (int, error) {
	return rw(bs)
}

func (c *connManager) accept(conn *net.UDPConn, addr *net.UDPAddr, cr *ClientRequest, _ Lister) {
	// TODO: find requested file and wrap into io.Reader
	// or send err if not found

	ik := key(addr)
	ackChan := make(chan *Acknowledgement)

	c.mux.Lock()
	if _, ok := c.conns[ik]; ok {
		// TODO: Conn already exists, do nothing, maybe send error to client?
		return
	}
	c.conns[ik] = &connection{
		ch: ackChan,
		sock: responseWriter(func(bs []byte) (int, error) {
			return conn.WriteTo(bs, addr)
		}),
	}
	c.mux.Unlock()

	sendData(ackChan, cr, nil)
}

func sendData(ackChan <-chan *Acknowledgement, cr *ClientRequest, _ []io.ReadSeeker) {
	// TODO: send data and handle ACKs
	// this may be a good place for heavy things like congestion control
}

func (c *connManager) handle(addr *net.UDPAddr, ack *Acknowledgement) {
	ik := key(addr)

	c.mux.Lock()
	conn, ok := c.conns[ik]
	if !ok {
		// TODO send error conn not found
	}
	c.mux.Unlock()

	conn.ch <- ack
}

func (c *connManager) close(addr *net.UDPAddr) {
	ik := key(addr)

	c.mux.Lock()
	conn := c.conns[ik]
	delete(c.conns, ik)
	c.mux.Unlock()

	close(conn.ch)
}

type directory string

func (d directory) List() ([]os.FileInfo, error) {
	return ioutil.ReadDir(string(d))
}

func DirectoryLister(dir string) directory {
	return directory(dir)
}
