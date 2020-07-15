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
	connMgr *connManager
	Conn    connection
}

func NewServer(l Lister) *Server {
	s := &Server{
		SRC: l,
		connMgr: &connManager{
			conns: make(map[string]*clientConnection),
		},
		Conn: NewUDPConnection(),
	}

	s.Conn.handle(msgClientRequest, handlerFunc(s.handleRequest))
	s.Conn.handle(msgClientAck, handlerFunc(s.handleACK))
	s.Conn.handle(msgClose, handlerFunc(s.handleClose))

	return s
}

func (s *Server) Listen(host string) error {
	cancel, err := s.Conn.listen(host)
	if err != nil {
		return err
	}
	defer cancel()
	return s.Conn.receive()
}

func (s *Server) handleRequest(w io.Writer, p *packet) {
	cr := &ClientRequest{}
	err := cr.UnmarshalBinary(p.data)
	if err != nil {
		// TODO: Close connection?
	}
	s.accept(w, p.remoteAddr, cr)
}

func (s *Server) handleACK(_ io.Writer, p *packet) {
	ack := &ClientAck{
		ackNumber: p.ackNum,
	}
	err := ack.UnmarshalBinary(p.data)
	if err != nil {
		// TODO: Close connection?
		log.Println("failed to parse ack")
	}
	s.connMgr.handle(p.remoteAddr, ack)
}

func (s *Server) handleClose(_ io.Writer, p *packet) {
	cl := CloseConnection{}
	err := cl.UnmarshalBinary(p.data)
	if err != nil {
		// TODO What now?
	}
	s.connMgr.close(p.remoteAddr)
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
func (s *Server) accept(w io.Writer, addr *net.UDPAddr, cr *ClientRequest) {
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
	s.connMgr.add(w, addr, cr, rss)
}

type clientConnection struct {
	rtt    time.Duration
	ch     chan *ClientAck
	socket io.Writer
}

type connManager struct {
	mux   sync.Mutex
	conns map[string]*clientConnection
}

func key(ip *net.UDPAddr) string {
	return fmt.Sprintf("%v:%v", ip.IP, ip.Port)
}

func (c *connManager) add(w io.Writer, addr *net.UDPAddr, cr *ClientRequest, rss []response) {
	// TODO: find requested file and wrap into io.Reader
	// or send err if not found

	ik := key(addr)
	ackChan := make(chan *ClientAck)
	newConn := &clientConnection{
		ch:     ackChan,
		socket: w,
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

func (c *clientConnection) sendData(ackChan <-chan *ClientAck, cr *ClientRequest, rss []response) {
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
		c.rtt = 1 * time.Second
		ticker := time.NewTicker(1 * time.Second)
		timeout := time.NewTimer(3 * c.rtt) //TODO: Adjust timeout duration

		counter := uint32(0)
		maxTransmission := 20000 + cr.maxTransmissionRate
		lastAck := uint8(0)

		for {
			log.Printf("server send loop budget counter: %v\n", counter)
			// this blocks when maxTransmissionRate is already used
			if counter >= maxTransmission {
				select {
				case <-ticker.C:
					counter = 0
				case ack := <-ackChan:
					//maxTransmission = ack.maxTransmissionRate
					lastAck = ack.ackNumber
					log.Printf("received ack while waiting for budget: %v, with resend entries:\n%v\n", lastAck, ack.resendEntries)
					timeout = time.NewTimer(3 * c.rtt)
					// TODO: schedule resends

					// continue to recheck maxTransmission capacity
				}
			}

			for {
				select {
				case bm, more := <-ch:
					if !more {
						// TODO: Cleanup?
						return
					}

					p, ok := bm.(ServerPayload)
					var err error
					if ok {
						p.ackNumber = lastAck
						err = sendTo(c.socket, p)
					} else {
						err = sendTo(c.socket, bm)
					}
					if err != nil {
						// TODO: What now? retry vs. close?
					}
					counter++

				case ack := <-ackChan:
					//maxTransmission = ack.maxTransmissionRate
					lastAck = ack.ackNumber
					log.Printf("received ack: %v, with resend entries:\n%v\n", lastAck, ack.resendEntries)
					timeout = time.NewTimer(3 * c.rtt)
					// TODO: schedule resends and drop acked bytes

				case <-ticker.C:
					counter = 0

				case <-timeout.C:
					// TODO: Cleanup channels etc.
					// TODO: Update (extend) timeout when acks arrive
					log.Println("connection timed out")
					return
				}
				if counter >= maxTransmission {
					break
				}
			}
		}
	}()

	for i := range cr.files {
		smd := ServerMetaData{
			status:    MetaDataStatus(rss[i].status),
			fileIndex: uint16(i),
			size:      rss[i].size,
		}
		b := buffer{
			smd:    &smd,
			hasher: md5.New(), // TODO: Make hash version configurable (including variable smd hash field sizes?)
			data:   []ServerPayload{},
		}
		buffers = append(buffers, b)

		log.Printf("reading file of size %v", rss[i].size)
		for j := uint64(0); j < smd.size; j += 1024 {
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
