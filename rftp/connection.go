package rftp

import (
	"encoding"
	"io"
	"net"
)

type UDPRequester struct {
	conn *net.UDPConn
}

func (u *UDPRequester) Dial(host string) error {
	addr, err := net.ResolveUDPAddr("udp", host)
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, addr)

	if err != nil {
		return err
	}

	u.conn = conn
	return nil
}

func (u *UDPRequester) Request(host string, m encoding.BinaryMarshaler) (encoding.BinaryUnmarshaler, error) {

	msg, err := m.MarshalBinary()
	if err != nil {
		return nil, err
	}

	err = u.Dial(host)
	if err != nil {
		return nil, err
	}

	err = send(msg, u.conn)

	if err != nil {
		return nil, err
	}

	// TODO: now receive the file request response packet
	// and then receive all the data packets and send corresponding acks

	r := make([]byte, 1024)
	_, _, err = u.conn.ReadFromUDP(r)

	if err != nil {
		return nil, err
	}
	return nil, nil
}

func send(data []byte, w io.Writer) error {
	_, err := w.Write(data)
	if err != nil {
		return err
	}
	return nil
}
