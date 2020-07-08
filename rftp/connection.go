package rftp

import (
	"encoding"
	"fmt"
	"io"
	"log"
	"net"
)

type packet struct {
	os         []option
	data       []byte
	remoteAddr *net.UDPAddr
}

type handlerFunc func(io.Writer, *packet)

func (h handlerFunc) handle(w io.Writer, p *packet) {
	h(w, p)
}

type packetHandler interface {
	handle(io.Writer, *packet)
}

type connection interface {
	handle(msgType uint8, h packetHandler)
	receive() error
	listen(host string) (func(), error)
	connectTo(host string) error
	send(msg encoding.BinaryMarshaler) error
}

type udpConnection struct {
	socket     *net.UDPConn
	handlers   map[uint8]packetHandler
	bufferSize int
}

type responseWriter func([]byte) (int, error)

func (rw responseWriter) Write(bs []byte) (int, error) {
	return rw(bs)
}

func NewUdpConnection() *udpConnection {
	return &udpConnection{
		handlers:   make(map[uint8]packetHandler),
		bufferSize: 1024,
	}
}

func (c *udpConnection) handle(msgType uint8, h packetHandler) {
	c.handlers[msgType] = h
}

func (c *udpConnection) receive() error {
	for {
		msg := make([]byte, c.bufferSize)
		n, addr, err := c.socket.ReadFromUDP(msg)
		if err != nil {
			// TODO: check error and maybe stop listening and shutdown
			log.Printf("discarded packet due to error: %v", err)
			break
		}

		header := &MsgHeader{}
		if err := header.UnmarshalBinary(msg); err != nil {
			// TODO: Is log, drop packet and continue enough? Maybe send close?
			log.Printf("error while unmarshalling packet header: %v\n", err)
			continue
		}

		rw := responseWriter(func(bs []byte) (int, error) {
			return c.socket.WriteTo(bs, addr)
		})
		p := &packet{
			os:         header.options,
			data:       msg[header.hdrLen:n],
			remoteAddr: addr,
		}
		go c.handlers[header.msgType].handle(rw, p)
	}

	return nil
}

func (c *udpConnection) listen(host string) (func(), error) {
	addr, err := net.ResolveUDPAddr("udp4", host)
	if err != nil {
		return nil, err
	}

	log.Printf("address: %v\n", addr)

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}
	c.socket = conn

	return func() {
		conn.Close()
	}, nil
}

func (c *udpConnection) connectTo(host string) error {
	addr, err := net.ResolveUDPAddr("udp", host)
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, addr)

	if err != nil {
		return err
	}

	c.socket = conn
	return nil
}

func (c udpConnection) send(msg encoding.BinaryMarshaler) error {
	return sendTo(c.socket, msg)
}

func sendTo(writer io.Writer, msg encoding.BinaryMarshaler) error {
	header := MsgHeader{
		version:   0,
		optionLen: 0,
	}

	switch v := msg.(type) {
	case ClientRequest:
		header.msgType = msgClientRequest
	case ClientAck:
		header.msgType = msgClientAck
	case ServerMetaData:
		header.msgType = msgServerMetadata
	case ServerPayload:
		header.msgType = msgServerPayload
	case CloseConnection:
		header.msgType = msgClose
	default:
		return fmt.Errorf("unknown msg type %T", v)
	}

	hs, err := header.MarshalBinary()
	if err != nil {
		return err
	}
	bs, err := msg.MarshalBinary()
	if err != nil {
		return err
	}

	_, err = writer.Write(append(hs, bs...))

	return err
}

var testConnectionAddr = &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 1000}

type testConnection struct {
	handlers map[uint8]packetHandler
	sentChan chan interface{} // sent out by application
	cancel   chan bool
	recvChan chan []byte // content is delivered to application, i.e., the test should fill this
}

func newTestConnection() *testConnection {
	return &testConnection{
		handlers: make(map[uint8]packetHandler),
		sentChan: make(chan interface{}, 100),
		cancel:   make(chan bool, 1),
		recvChan: make(chan []byte, 100),
	}
}

func (c *testConnection) handle(msgType uint8, h packetHandler) {
	c.handlers[msgType] = h
}

func (c *testConnection) receive() error {
	rw := responseWriter(func(bs []byte) (n int, err error) {
		n = len(bs)
		header := &MsgHeader{}
		if err = header.UnmarshalBinary(bs); err != nil {
			// signal tests that this error occured?
			return n, nil
		}

		var msg encoding.BinaryUnmarshaler
		switch header.msgType {
		case msgClientRequest:
			msg = &ClientRequest{}
		case msgServerMetadata:
			msg = &ServerMetaData{}
		case msgServerPayload:
			msg = &ServerPayload{}
		case msgClientAck:
			msg = &ClientAck{}
		case msgClose:
			msg = &CloseConnection{}
		default:
			return n, nil
		}

		if err = msg.UnmarshalBinary(bs); err != nil {
			return n, nil
		}

		c.sentChan <- msg
		return n, nil
	})

	for {
		select {
		case <-c.cancel:
			return nil
		case msg := <-c.recvChan:
			header := &MsgHeader{}
			if err := header.UnmarshalBinary(msg); err != nil {
				return fmt.Errorf("error while unmarshalling packet header: %v\n", err)
			}

			p := &packet{
				os:         header.options,
				data:       msg[header.hdrLen:],
				remoteAddr: testConnectionAddr, // TODO: make configurable
			}
			go c.handlers[header.msgType].handle(rw, p)
		}
	}
	return nil
}

func (c *testConnection) listen(host string) (func(), error) {
	return func() {
		c.cancel <- true
	}, nil
}

func (c testConnection) connectTo(host string) error {
	return nil
}

func (c testConnection) send(msg encoding.BinaryMarshaler) error {
	c.sentChan <- msg
	return nil
}
