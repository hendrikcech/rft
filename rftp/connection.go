package rftp

import (
	"encoding"
	"fmt"
	"log"
	"net"
)

type handlerFunc func([]option, []byte)

func (h handlerFunc) handle(os []option, bs []byte) {
	h(os, bs)
}

type packetHandler interface {
	handle([]option, []byte)
}

type connection struct {
	conn *net.UDPConn

	handlers map[uint8]packetHandler

	bufferSize int
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

func (c *connection) receive() {
	for {
		msg := make([]byte, c.bufferSize)
		n, _, err := c.conn.ReadFromUDP(msg)
		if err != nil {
			// TODO: check error and maybe stop listening and shutdown
			log.Printf("discarded packet due to error: %v", err)
			continue
		}

		header := &MsgHeader{}
		if err := header.UnmarshalBinary(msg); err != nil {
			// TODO: Is log, drop packet and continue enough? Maybe send close?
			log.Printf("error while unmarshalling packet header: %v\n", err)
			continue
		}

		go c.handlers[header.msgType].handle(header.options, msg[header.hdrLen:n])
	}
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

	c.conn = conn
	return nil
}

func (c *connection) send(msg encoding.BinaryMarshaler) error {
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

	_, err = c.conn.Write(append(hs, bs...))

	return err
}

/*
func (u *UDPRequester) Request(host string, m encoding.BinaryMarshaler) (encoding.BinaryUnmarshaler, error) {

	hdr := &MsgHeader{
		version:   0,
		msgType:   msgClientRequest,
		optionLen: 0,
	}
	hs, err := hdr.MarshalBinary()
	if err != nil {
		return nil, err
	}
	msg, err := m.MarshalBinary()
	if err != nil {
		return nil, err
	}

	err = u.Dial(host)
	if err != nil {
		return nil, err
	}

	_, err = u.conn.Write(append(hs, msg...))
	if err != nil {
		return nil, err
	}

	// TODO: now receive the file request response packet
	// and then receive all the data packets and send corresponding acks
	packet := make([]byte, 2048)
	for {
		_, _, err = u.conn.ReadFromUDP(packet)
		if err != nil {
			return nil, err
		}

		hdr := MsgHeader{}
		err = hdr.UnmarshalBinary(packet)
		if err != nil {
			return nil, err
		}
		smd := ServerMetaData{}
		err = smd.UnmarshalBinary(packet[hdr.hdrLen:])
		if err != nil {
			return nil, err
		}

		log.Printf("received packet:\nhdr: %v\nval:%v\n", hdr, smd)
	}
}
*/
