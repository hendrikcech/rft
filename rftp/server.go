package rftp

import (
	"crypto/md5"
	"fmt"
	"hash"
	"io"
	"log"
	"net"
	"sort"
	"sync"
	"time"
)

type FileHandler func(name string) (*io.SectionReader, error)

type fileReader struct {
	index  uint16
	offset uint64
	sr     *io.SectionReader
	hasher hash.Hash
}

type clientConnection struct {
	rtt           time.Duration
	req           *clientRequest
	payload       chan *serverPayload
	resend        chan *serverPayload
	metadata      chan *serverMetaData
	ack           chan *clientAck
	reschedule    chan *clientAck
	resendDone    chan *serverPayload
	rescheduledAt map[uint64]time.Time
	cclose        chan *closeConnection
	socket        io.Writer

	cleaner cleaner

	metadataCache    map[uint16]*serverMetaData
	payloadCache     map[uint16]map[uint64]*serverPayload
	payloadCacheLock sync.Mutex
}

func (c *clientConnection) writeResponse() {
	log.Println("start writing response packets")
	lastAck := uint8(0)
	rateControl := &aimd{congRate: 1000}
	rateControl.start()
	defer rateControl.stop()

	handleAck := func(ack *clientAck) {
		lastAck = ack.ackNumber
		rateControl.onAck(ack)
		c.reschedule <- ack
		c.cleaner.refresh(5 * time.Second) // TODO: replace by 500 + RTT * 3 or something
	}

	closeChan := c.cleaner.subscribe()

	for !c.cleaner.closed() {
		var err error

		if rateControl.isAvailable() {
			select {
			case pl := <-c.resend:
				log.Printf("resending payload for file %v at offset %v with acknum: %v\n", pl.fileIndex, pl.offset, lastAck)
				pl.ackNumber = lastAck
				err = sendTo(c.socket, *pl)
				rateControl.onSend()
				c.resendDone <- pl
				continue

			case ack := <-c.ack:
				handleAck(ack)

			default:
			}
			select {
			case md := <-c.metadata:
				log.Printf(
					"sending metadata for file %v: status: %v, size: %v, checksum: %x\n",
					md.fileIndex,
					md.status,
					md.size,
					md.checkSum,
				)
				md.ackNum = lastAck
				c.metadataCache[md.fileIndex] = md
				err = sendTo(c.socket, *md)
				rateControl.onSend()

			case pl := <-c.payload:
				log.Printf("sending payload for file %v at offset %v with acknum: %v\n", pl.fileIndex, pl.offset, lastAck)
				pl.ackNumber = lastAck
				c.saveToCache(pl)
				err = sendTo(c.socket, *pl)
				rateControl.onSend()

			case ack := <-c.ack:
				handleAck(ack)

			case <-closeChan:
				return
			}
		} else {
			select {
			case <-rateControl.awaitAvailable():
				continue
			case ack := <-c.ack:
				handleAck(ack)
			case <-closeChan:
				return
			}
		}

		if err != nil {
			log.Println(err)
		}
	}
}

// TODO: Drop cached payloads. That's not trivial, because we don't have
// explicit acks per file, so we have to calculate it, to avoid keeping all
// files in the cache.
func (c *clientConnection) saveToCache(p *serverPayload) {
	c.payloadCacheLock.Lock()
	defer c.payloadCacheLock.Unlock()
	_, ok := c.payloadCache[p.fileIndex]
	if !ok {
		c.payloadCache[p.fileIndex] = make(map[uint64]*serverPayload)
	}

	c.payloadCache[p.fileIndex][p.offset] = p
}

func (c *clientConnection) getFromCache(file uint16, offset uint64) (*serverPayload, bool) {
	c.payloadCacheLock.Lock()
	defer c.payloadCacheLock.Unlock()

	if c, ok := c.payloadCache[file]; ok {
		if p, ok := c[offset]; ok {
			return p, true
		}
	}
	return nil, false
}

func (c *clientConnection) rescheduler() {
	closeChan := c.cleaner.subscribe()
	resendScheduled := map[uint16]map[uint64]struct{}{}

	for {
		select {
		case <-closeChan:
			return
		case p := <-c.resendDone:
			delete(resendScheduled[p.fileIndex], p.offset)
		case ack := <-c.reschedule:
			// use a map to avoid duplicates in metadata resend entries
			metadata := map[uint16]struct{}{}
			if ack.metaDataMissing {
				metadata[ack.fileIndex] = struct{}{}
			}

			sort.Sort(&ack.resendEntries)
			//log.Printf("rescheduling sorted ack: %v\n", ack)

			if len(ack.resendEntries) <= 0 {
				if p, ok := c.getFromCache(ack.fileIndex, ack.offset); ok {
					c.resend <- p
					log.Printf("rescheduled payload for file: %v at offset: %v\n", p.fileIndex, p.offset)
				}
			}
			for i, re := range ack.resendEntries {
				if ack.maxTransmissionRate > 0 && uint32(i) > ack.maxTransmissionRate {
					break
				}
				if re.length == 0 {
					metadata[re.fileIndex] = struct{}{}
				}
				if _, exists := resendScheduled[re.fileIndex]; !exists {
					resendScheduled[re.fileIndex] = make(map[uint64]struct{})
				}
				if _, ok := resendScheduled[re.fileIndex][re.offset]; !ok {
					resendScheduled[re.fileIndex][re.offset] = struct{}{}

					if p, ok := c.getFromCache(re.fileIndex, re.offset); ok {
						if re.length == 0 {
							c.resend <- p
							log.Printf("rescheduled payload for file: %v at offset: %v\n", p.fileIndex, p.offset)
						}

						for i := uint64(0); i < uint64(re.length); i++ {
							if p, ok := c.getFromCache(re.fileIndex, re.offset+i); ok {
								c.resend <- p
								log.Printf("rescheduled payload for file: %v at offset: %v\n", p.fileIndex, p.offset)
							} else {
								log.Printf("didn't find resend entry in cache: %v\n", re.offset+i)
								break
								// TODO:
								// re-read from sectionReader, this isn't trivial either
								// because we may have to avoid concurrent reads on the files
							}
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
	}
}

func (c *clientConnection) getResponse(fh FileHandler) {
	if fh == nil {
		// TODO Send error file not available
	}

	c.payload = make(chan *serverPayload, 1024*1024)
	c.resend = make(chan *serverPayload, 1024*1024)
	c.metadata = make(chan *serverMetaData, len(c.req.files))
	c.reschedule = make(chan *clientAck, 1024)
	c.resendDone = make(chan *serverPayload, 1024*1024)

	go c.writeResponse()
	go c.rescheduler()

	srs := []fileReader{}
	for i, fr := range c.req.files {
		r, err := fh(fr.fileName)
		if err != nil {
			// TODO
			// send err metadata
		}
		sr := fileReader{
			index:  uint16(i),
			sr:     r,
			hasher: md5.New(),
		}
		srs = append(srs, sr)

		// Copy pre offset bytes to hasher
		n, err := io.CopyN(sr.hasher, sr.sr, int64(fr.offset*1024))
		if err != nil || n != int64(fr.offset) {
			// TODO
			// report read error
		}
	}

	closeChan := c.cleaner.subscribe()

	for _, fr := range srs {
		if c.cleaner.closed() {
			return
		}

		if fr.sr == nil {
			c.metadata <- &serverMetaData{fileIndex: fr.index, status: fileNotExistent}
			continue
		}
		if fr.sr.Size() == 0 {
			c.metadata <- &serverMetaData{fileIndex: fr.index, status: fileEmpty}
			continue
		}

		done := false
		off := int64(0)
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
			p := &serverPayload{
				fileIndex: fr.index,
				data:      buf[:n],
				offset:    uint64(off),
			}
			off++
			select {
			case c.payload <- p:
			case <-closeChan:
				return
			}
		}

		m := &serverMetaData{fileIndex: fr.index, size: uint64(fr.sr.Size())}
		copy(m.checkSum[:], fr.hasher.Sum(nil)[:16])
		c.metadata <- m
	}
}

func key(ip *net.UDPAddr) string {
	return fmt.Sprintf("%v:%v", ip.IP, ip.Port)
}

type cleaner struct {
	closeLock   sync.RWMutex
	subs        []chan struct{}
	closedState bool

	timeoutLock sync.Mutex
	deadline    time.Time

	cb func()
}

func (c *cleaner) close() {
	c.closeLock.Lock()
	defer c.closeLock.Unlock()
	if c.closedState {
		return
	}
	c.closedState = true
	for _, sub := range c.subs {
		sub <- struct{}{}
		close(sub)
	}
	c.cb()
}

func (c *cleaner) closed() bool {
	c.closeLock.RLock()
	defer c.closeLock.RUnlock()
	return c.closedState
}

func (c *cleaner) refresh(d time.Duration) {
	c.timeoutLock.Lock()
	defer c.timeoutLock.Unlock()
	c.deadline = time.Now().Add(d)
}

func (c *cleaner) checkTimeout() {
	c.timeoutLock.Lock()
	defer c.timeoutLock.Unlock()
	if time.Now().After(c.deadline) {
		c.close()
	} else if !c.closed() {
		time.AfterFunc(time.Until(c.deadline), c.checkTimeout)
	}
}

func (c *cleaner) subscribe() <-chan struct{} {
	c.closeLock.Lock()
	defer c.closeLock.Unlock()
	new := make(chan struct{}, 1)
	c.subs = append(c.subs, new)
	if c.closedState {
		new <- struct{}{}
		close(new)
	}
	return new
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

func (s *Server) Addr() net.Addr {
	return s.Conn.addr()
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

	log.Printf("running server on addr '%v'\n", s.Conn.addr())
	return s.Conn.receive()
}

func (s *Server) SetFileHandler(fh FileHandler) {
	s.fh = fh
}

type unreliableWriter struct {
	breakTime  time.Time
	returnTime time.Time
	w          io.Writer
}

func (u *unreliableWriter) Write(p []byte) (n int, err error) {
	t := time.Now()
	if u.breakTime.After(t) {
		return u.w.Write(p)
	}
	if u.breakTime.After(t) && u.returnTime.Before(t) {
		return u.w.Write(p)
	}
	return len(p), nil
}

func getUnreliableWriter(w io.Writer, x, y time.Duration) io.Writer {
	return &unreliableWriter{
		breakTime:  time.Now().Add(x),
		returnTime: time.Now().Add(y),
		w:          w,
	}
}

func (s *Server) handleRequest(w io.Writer, p *packet) {

	// Uncomment to get a network failure between x and y
	//x := 5 * time.Second
	//y := 20 * time.Second
	//w = getUnreliableWriter(w, x, y)

	cr := &clientRequest{}
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
			ack:    make(chan *clientAck, 1024),
			cclose: make(chan *closeConnection),
			socket: w,
			req:    cr,

			cleaner: cleaner{cb: func() {
				s.clientMux.Lock()
				defer s.clientMux.Unlock()
				delete(s.clients, key)
				log.Printf("Conn %v closed. Current number of connections: %v\n", key, len(s.clients))
			}},

			payloadCache:  make(map[uint16]map[uint64]*serverPayload),
			metadataCache: make(map[uint16]*serverMetaData),
		}
		s.clients[key] = c
		go c.getResponse(s.fh)
		c.cleaner.refresh(5 * time.Second)
		c.cleaner.checkTimeout()
	} else {
		// TODO: send close, because duplicate connection request
	}
}

func (s *Server) handleACK(_ io.Writer, p *packet) {
	ack := &clientAck{}
	err := ack.UnmarshalBinary(p.data)
	if err != nil {
		// TODO: Close connection?
		log.Println("failed to parse ack")
	}
	ack.ackNumber = p.ackNum
	key := key(p.remoteAddr)
	s.clientMux.Lock()
	defer s.clientMux.Unlock()
	if conn, ok := s.clients[key]; ok {
		conn.ack <- ack
	}
}

func (s *Server) handleClose(_ io.Writer, p *packet) {
	cl := closeConnection{}
	err := cl.UnmarshalBinary(p.data)
	if err != nil {
		// TODO What now?
		log.Println("failed to parse close")
	}

	log.Printf("connection closed: %s\n", cl.reason.String())
	// TODO: clean up state
}
