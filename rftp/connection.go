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

type connection struct {
	socket *net.UDPConn

	handlers map[uint8]packetHandler

	bufferSize int
}

type responseWriter func([]byte) (int, error)

func (rw responseWriter) Write(bs []byte) (int, error) {
	return rw(bs)
}

func newConnection() *connection {
	return &connection{
		handlers:   make(map[uint8]packetHandler),
		bufferSize: 1024,
	}
}

func (c *connection) handle(msgType uint8, h packetHandler) {
	c.handlers[msgType] = h
}

func (c *connection) receive() error {
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

func (c *connection) listen(host string) (func(), error) {
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

func (c *connection) connectTo(host string) error {
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
