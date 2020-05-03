package rftp

import (
	"net"
	"os"
)

type Lister interface {
	List() ([]os.FileInfo, error)
}

type Server struct {
	SRC Lister
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
		_, addr, err := conn.ReadFromUDP(msg)
		if err != nil {
			return err
		}

		conn.WriteToUDP(msg, addr)
	}
}
