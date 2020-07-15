// +build s2

package rftp

import (
	"crypto/md5"
	"fmt"
	"hash"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type FileHandler func(name string, offset uint64) *io.SectionReader

type fileReader struct {
	index  uint16
	sr     *io.SectionReader
	hasher hash.Hash
}

type clientConnection struct {
	rtt      time.Duration
	req      *ClientRequest
	payload  chan *ServerPayload
	metadata chan *ServerMetaData
	ack      chan *ClientAck
	cclose   chan *CloseConnection
	socket   io.Writer

	metadataCache map[uint16]*ServerMetaData
	payloadCache  map[uint16]map[uint64]*ServerPayload
}

func (c *clientConnection) writeResponse() {
	log.Println("start writing response packets")
	lastAck := uint8(0)
	for {
		var err error

		select {
		case md := <-c.metadata:
			log.Printf("sending metadata for file %v: status: %v, size: %v, checksum: %x\n", md.fileIndex, md.status, md.size, md.checkSum)
			md.ackNum = lastAck
			c.metadataCache[md.fileIndex] = md
			err = sendTo(c.socket, *md)

		case pl := <-c.payload:
			log.Printf("sending payload for file %v at offset %v\n", pl.fileIndex, pl.offset)
			pl.ackNumber = lastAck
			c.saveToCache(pl)
			err = sendTo(c.socket, *pl)

		case ack := <-c.ack:
			lastAck = ack.ackNumber
			c.reschedule(ack)
		}

		if err != nil {
			log.Println(err)
		}
	}
}

// TODO: Drop cached payloads. That's not trivial, because we don't have
// explicit acks per file, so we have to calculate it, to avoid keeping all
// files in the cache.
func (c *clientConnection) saveToCache(p *ServerPayload) {
	_, ok := c.payloadCache[p.fileIndex]
	if !ok {
		c.payloadCache[p.fileIndex] = make(map[uint64]*ServerPayload)
	}

	c.payloadCache[p.fileIndex][p.offset] = p
}

func (c *clientConnection) reschedule(ack *ClientAck) {
	// use a map to avoid duplicates in metadata resend entries
	metadata := map[uint16]struct{}{}
	if ack.status != 0 {
		metadata[ack.fileIndex] = struct{}{}
	}

	for _, re := range ack.resendEntries {
		if re.length == 0 {
			metadata[re.fileIndex] = struct{}{}
		}
		if m, ok := c.payloadCache[re.fileIndex]; ok {
			for i := uint64(0); i < uint64(re.length); i++ {
				if p, ok := m[re.offset+i]; ok {
					c.payload <- p
					log.Printf("rescheduled payload for file: %v at offset: %v\n", p.fileIndex, p.offset)
				} else {
					log.Println("didn't find resend entry in cache")
					// TODO:
					// re-read from sectionReader, this isn't trivial either
					// because we may have to avoid concurrent reads on the files
				}
			}
		}
	}

	// resend metadata
	for k := range metadata {
		if m, ok := c.metadataCache[k]; ok {
			log.Printf("rescheduled metadata for file: %v\n", k)
			c.metadata <- m
		}
	}
}

func (c *clientConnection) getResponse(fh FileHandler) {
	if fh == nil {
		// TODO Send error file not available
	}

	c.payload = make(chan *ServerPayload, 1024)
	c.metadata = make(chan *ServerMetaData, len(c.req.files))

	go c.writeResponse()

	srs := []fileReader{}
	for i, fr := range c.req.files {
		srs = append(srs, fileReader{
			index:  uint16(i),
			sr:     fh(fr.fileName, fr.offset),
			hasher: md5.New(),
		})
	}

	for _, fr := range srs {
		off := int64(0)
		done := false
		for !done {
			buf := make([]byte, 1024)
			n, err := fr.sr.ReadAt(buf, 1024*off)
			if err == io.EOF {
				done = true
			}
			if err != nil {
				log.Printf("error, on reading file: %v\n", err)
			}
			_, err = fr.hasher.Write(buf[:n])
			if err != nil {
				log.Printf("failed to write to hash: %v\n", err)
			}
			p := &ServerPayload{
				fileIndex: fr.index,
				data:      buf[:n],
				offset:    uint64(off),
			}
			off++
			c.payload <- p
		}
		m := &ServerMetaData{fileIndex: fr.index, size: uint64(fr.sr.Size())}
		copy(m.checkSum[:], fr.hasher.Sum(nil)[:16])
		c.metadata <- m
	}
}

func key(ip *net.UDPAddr) string {
	return fmt.Sprintf("%v:%v", ip.IP, ip.Port)
}

type Server struct {
	Conn connection
	fh   FileHandler

	clients   map[string]*clientConnection
	clientMux sync.Mutex
}

func NewServer() *Server {
	s := &Server{
		Conn:    NewUDPConnection(),
		clients: make(map[string]*clientConnection),
	}

	return s
}

func (s *Server) Listen(host string) error {
	s.Conn.handle(msgClientRequest, handlerFunc(s.handleRequest))
	s.Conn.handle(msgClientAck, handlerFunc(s.handleACK))
	s.Conn.handle(msgClose, handlerFunc(s.handleClose))

	cancel, err := s.Conn.listen(host)
	if err != nil {
		return err
	}
	defer cancel()
	return s.Conn.receive()
}

func (s *Server) SetFileHandler(fh FileHandler) {
	s.fh = fh
}

func (s *Server) handleRequest(w io.Writer, p *packet) {
	cr := &ClientRequest{}
	err := cr.UnmarshalBinary(p.data)
	if err != nil {
		// TODO: Close connection?
		log.Println("failed to parse data")
	}

	key := key(p.remoteAddr)
	s.clientMux.Lock()
	defer s.clientMux.Unlock()
	if _, ok := s.clients[key]; !ok {
		c := &clientConnection{
			ack:    make(chan *ClientAck, 1024),
			cclose: make(chan *CloseConnection),
			socket: w,
			req:    cr,

			payloadCache:  make(map[uint16]map[uint64]*ServerPayload),
			metadataCache: make(map[uint16]*ServerMetaData),
		}
		s.clients[key] = c
		go c.getResponse(s.fh)
	} else {
		// send close, because duplicate connection request
	}
}

func (s *Server) handleACK(_ io.Writer, p *packet) {
	ack := &ClientAck{}
	err := ack.UnmarshalBinary(p.data)
	if err != nil {
		// TODO: Close connection?
		log.Println("failed to parse ack")
	}
	ack.ackNumber = p.ackNum
	key := key(p.remoteAddr)
	s.clientMux.Lock()
	defer s.clientMux.Unlock()
	if _, ok := s.clients[key]; !ok {
		// drop packet
		return
	}
	s.clients[key].ack <- ack
}

func (s *Server) handleClose(_ io.Writer, p *packet) {
	cl := CloseConnection{}
	err := cl.UnmarshalBinary(p.data)
	if err != nil {
		// TODO What now?
		log.Println("failed to parse close")
	}
}
