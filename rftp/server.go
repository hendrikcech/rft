package rftp

import (
	"crypto/md5"
	"encoding"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

type Lister interface {
	List() ([]os.FileInfo, error)
}

type Server struct {
	SRC     Lister
	sock    *net.UDPConn
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
	addr, err := net.ResolveUDPAddr("udp4", host)
	if err != nil {
		return err
	}

	log.Printf("address: %v\n", addr)

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return err
	}
	s.sock = conn

	defer conn.Close()

	log.Printf("start listening on %v\n", conn.LocalAddr().String())

	for {
		msg := make([]byte, 1024)
		n, addr, err := conn.ReadFromUDP(msg)
		if err != nil {
			return err
		}
		//		log.Printf("received packet of length %v: \n%v\n", n, hex.Dump(msg))

		go s.handlePacket(conn, addr, n, msg)
	}
}

func (s *Server) handlePacket(conn *net.UDPConn, addr *net.UDPAddr, length int, packet []byte) {
	header := &MsgHeader{}
	if err := header.UnmarshalBinary(packet); err != nil {
		// TODO: Drop connection
		log.Printf("error while unmarshalling packet header: %v\n", err)
	}

	switch header.msgType {
	case msgClientRequest:
		cr := &ClientRequest{}
		err := cr.UnmarshalBinary(packet[header.hdrLen:])
		if err != nil {
			// TODO: Drop connection
			log.Printf("error while unmarshalling client request: %v\n", err)
		}
		s.accept(addr, cr)

	case msgClientAck:
		ack := &ClientAck{}
		ack.UnmarshalBinary(packet[header.hdrLen:])
		s.connMgr.handle(addr, ack)

	case msgClose:
		s.connMgr.close(addr)

	default:
		// TODO: Drop connection
	}
}

func findFileIn(name string, files []os.FileInfo) os.FileInfo {
	for _, f := range files {
		if f.Name() == name {
			return f
		}
	}
	return nil
}

// TODO accept could likely be a lot smarter.  Some caching might be a good
// idea.  A bit more complex but maybe useful addition would be to have the user
// specify a handler for any given ClientRequest. Therefore we could replace the
// Lister by a map of handlers, which match a certain ClientRequest
// The handler would likely need a writer to write a response to, but it is
// important that the underlying type implements an io.ReadSeeker, which is
// needed for retransmission (see sendData or flow and congestion control spec)
func (s *Server) accept(addr *net.UDPAddr, cr *ClientRequest) {
	fs, err := s.SRC.List()
	if err != nil {
		// TODO: reject all
		log.Printf("error while listing src files: %v\n", err)
	}

	rss := []response{}
	for _, rf := range cr.files {
		if f := findFileIn(rf.fileName, fs); f != nil {
			rs, err := os.Open(f.Name())
			if err != nil {
				rss = append(rss, response{
					rs:     nil,
					size:   0,
					status: 0x03,
				})
				continue
			}

			size := f.Size()
			if size <= 0 {
				rss = append(rss, response{
					rs:     nil,
					size:   0,
					status: 0x02,
				})
			}

			rss = append(rss, response{
				rs:     rs,
				size:   uint64(f.Size()),
				status: 0,
			})
		} else {
			rss = append(rss, response{
				rs:     nil,
				size:   0,
				status: 0x01,
			})
		}

	}

	log.Println("adding connection")
	s.connMgr.add(s.sock, addr, cr, rss)
}

type connection struct {
	ch   chan *ClientAck
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

func (c *connManager) add(conn *net.UDPConn, addr *net.UDPAddr, cr *ClientRequest, rss []response) {
	// TODO: find requested file and wrap into io.Reader
	// or send err if not found

	ik := key(addr)
	ackChan := make(chan *ClientAck)
	newConn := &connection{
		ch: ackChan,
		sock: responseWriter(func(bs []byte) (int, error) {
			//			log.Printf("sending bytes to %v: %v\n", addr, bs)
			return conn.WriteToUDP(bs, addr)
		}),
	}

	c.mux.Lock()
	if _, ok := c.conns[ik]; ok {
		// TODO: Conn already exists, do nothing, maybe send error to client?
		return
	}
	c.conns[ik] = newConn
	c.mux.Unlock()

	newConn.sendData(ackChan, cr, rss)
}

type response struct {
	rs     io.ReadSeeker
	status uint8
	size   uint64
}

func (c *connection) buildAndSend(bm encoding.BinaryMarshaler, lastAck uint8) {
	var msgT uint8
	switch v := bm.(type) {
	case ServerMetaData:
		msgT = msgServerMetadata
	case ServerPayload:
		msgT = msgServerPayload
		v.ackNumber = lastAck
	default:
		// TODO: should never happen
	}

	header := MsgHeader{
		version:   0,
		msgType:   msgT,
		optionLen: 0,
	}

	hs, err := header.MarshalBinary()
	if err != nil {
		// TODO: no idea what now...
		// cancel and close connection?
	}

	bs, err := bm.MarshalBinary()
	if err != nil {
		// TODO: no idea what now...
		// cancel and close connection?
	}
	log.Printf("sending packet: %v:%v\n", header, string(bs))
	_, err = c.sock.Write(append(hs, bs...))
}

func (c *connection) sendData(ackChan <-chan *ClientAck, cr *ClientRequest, rss []response) {
	// TODO: send data and handle ACKs
	// this may be a good place for heavy things like congestion control

	type buffer struct {
		smd    *ServerMetaData
		hasher hash.Hash
		data   []ServerPayload
	}
	var buffers []buffer
	ch := make(chan encoding.BinaryMarshaler, 1024)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		timeout := time.NewTimer(30 * time.Second) //TODO: Adjust timeout duration

		counter := uint32(0)
		maxTransmission := 10 + cr.maxTransmissionRate
		lastAck := uint8(0)

		for {
			// this blocks when maxTransmissionRate is already used
			if counter >= maxTransmission {
				select {
				case <-ticker.C:
					counter = 0
				case ack := <-ackChan:
					maxTransmission = ack.maxTransmissionRate
					lastAck = ack.ackNumber
					// TODO: schedule resends

					// continue to recheck maxTransmission capacity
					continue
				}
			}

			select {
			case bm, more := <-ch:
				if !more {
					// TODO: Cleanup?
					return
				}

				c.buildAndSend(bm, lastAck)
				counter++

			case ack := <-ackChan:
				maxTransmission = ack.maxTransmissionRate
				lastAck = ack.ackNumber
				// TODO: schedule resends and drop acked bytes

			case <-ticker.C:
				counter = 0

			case <-timeout.C:
				// TODO: Cleanup channels etc.
				// TODO: Update (extend) timeout when acks arrive
				log.Println("connection timed out")
				return
			}
		}
	}()

	for i := range cr.files {
		smd := ServerMetaData{
			status:    rss[i].status,
			fileIndex: uint16(i),
			size:      rss[i].size,
		}
		b := buffer{
			smd:    &smd,
			hasher: md5.New(), // TODO: Make hash version configurable (including variable smd hash field sizes?)
			data:   []ServerPayload{},
		}
		buffers = append(buffers, b)

		for j := uint64(0); j < smd.size; j += 1024 {
			log.Printf("reading file of size %v", rss[i].size)
			buf := make([]byte, 1024)
			off, err := rss[i].rs.Seek(int64(j), io.SeekStart)
			if err != nil {
				log.Printf("error at seek: %v", err)
			}
			log.Printf("read at offset %v", off)
			n, err := rss[i].rs.Read(buf)
			if err != nil {
				// TODO
			}
			log.Printf("read %v bytes from file", n)
			_, err = b.hasher.Write(buf[:n])
			if err != nil {
				// TODO
			}
			// io.CopyN(&buf, io.TeeReader(rss[i].rs, b.hasher), 1024) Copy
			// doesn't work because it doesn't seem to care about seek (how can
			// it though...)
			payload := ServerPayload{
				fileIndex: uint16(i),
				offset:    j / 1024,
				data:      buf[:n],
			}
			b.data = append(b.data, payload)
			log.Printf("send payload part %d\n", payload.offset)
			ch <- payload
		}
		copy(smd.checkSum[:], b.hasher.Sum(nil)[:16])
		log.Printf("checksum of file %v\n", b.smd.checkSum)
		log.Printf("hex checksum of file %x\n", b.hasher.Sum(nil))
		ch <- smd
	}
}

func (c *connManager) handle(addr *net.UDPAddr, ack *ClientAck) {
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
