package rftp

import (
	"encoding"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

var defaultClient = Client{}

func Request(host string, files []string) ([]Result, error) {
	return defaultClient.Request(NewUdpConnection(), host, files)
}

type Requester interface {
	Request(string, encoding.BinaryMarshaler) (encoding.BinaryUnmarshaler, error)
}

type Result struct {
	lock       *sync.Mutex
	buffer     []ServerPayload // TODO: Replace by priority queue
	pipeReader io.Reader
	pipeWriter io.Writer
	payload    chan *ServerPayload
	pointer    uint64 // Next byte position to write out
	offset     uint64 // Offset as defined by rft
	started    bool   // true if received at least 1 chunk for this file
	size       uint64
	checksum   [16]byte
	err        error
}

// TODO: After replacing buffer by a useful datastructure, calculate the
// complement of the buffered elements
func (r Result) getResendEntries() []ResendEntry {
	return []ResendEntry{}
}

func (r *Result) Read(p []byte) (n int, err error) {
	return r.pipeReader.Read(p)
}

type Client struct {
	results    []Result
	resultLock *sync.Mutex
	smd        chan *ServerMetaData
	payload    chan *ServerPayload
	ackNum     chan uint8
}

func (c *Client) Request(conn connection, host string, files []string) ([]Result, error) {
	fs := []FileDescriptor{}
	for i, f := range files {
		fs = append(fs, FileDescriptor{0, f})
		r, w := io.Pipe()
		c.results = append(c.results, Result{
			buffer:     []ServerPayload{},
			pipeReader: r,
			pipeWriter: w,
			payload:    make(chan *ServerPayload, 1024),
		})
		go c.writerToApp(i)
	}

	c.smd = make(chan *ServerMetaData, len(fs))
	c.payload = make(chan *ServerPayload, 1024*len(fs))
	c.ackNum = make(chan uint8, 256)

	conn.handle(msgServerMetadata, c.ackNumHandler(handlerFunc(c.handleMetadata)))
	conn.handle(msgServerPayload, c.ackNumHandler(handlerFunc(c.handleServerPayload)))
	conn.handle(msgClose, c.ackNumHandler(handlerFunc(c.handleClose)))

	if err := conn.connectTo(host); err != nil {
		return nil, err
	}
	if err := conn.send(ClientRequest{
		maxTransmissionRate: 0,
		files:               fs,
	}); err != nil {
		return nil, err
	}

	go c.bufferResults()
	go conn.receive()
	go c.sendAcks(conn)

	var errStrings []string
	for smd := range c.smd {
		i := smd.fileIndex

		if smd.status != 0 {
			err := fmt.Errorf("error receiving file: %v", smd.status)
			errStrings = append(errStrings, err.Error())
			c.results[i].err = err
			close(c.results[i].payload)
			continue
		}

		c.results[i].size = smd.size
		c.results[i].checksum = smd.checkSum
	}

	if len(errStrings) > 0 {
		return c.results, fmt.Errorf(strings.Join(errStrings, ", "))
	}
	return c.results, nil
}

func (c *Client) ackNumHandler(hf handlerFunc) handlerFunc {
	return func(w io.Writer, p *packet) {
		c.ackNum <- p.ackNum
		hf(w, p)
	}
}

func (c *Client) sendAcks(conn connection) {

	nextAckNumber := uint8(1) // need to start with 1, to be able to distinguish between server header with no ACK number and our first ACK number

	ackSendMap := map[uint8]time.Time{}
	rttMap := map[uint8]time.Duration{}
	rtt := time.Duration(1 * time.Second) // TODO: calculate real rtt or use this as initial timeout?

	timeout := time.NewTimer(rtt)
	for {
		select {
		case an := <-c.ackNum:
			rttMap[an] = time.Since(ackSendMap[an])

		case <-timeout.C:
			res, fi, off := c.getAckData()

			ack := ClientAck{
				ackNumber:           nextAckNumber,
				maxTransmissionRate: 0,
				fileIndex:           fi,
				offset:              off,
				resendEntries:       res,
			}
			conn.send(ack)

			ackSendMap[nextAckNumber] = time.Now()
			nextAckNumber = (nextAckNumber + 1) % 255
			rtt = avg(rttMap)
			timeout = time.NewTimer(rtt / 4)
		}
	}
}

func (c *Client) getAckData() (res []ResendEntry, fi uint16, off uint64) {
	c.resultLock.Lock()
	defer c.resultLock.Unlock()

	for i := 0; i <= len(c.results); i++ {
		c.results[i].lock.Lock()
		if c.results[i].started {
			fi = uint16(i)
			off = c.results[i].offset
			res = append(res, c.results[i].getResendEntries()...)
		}
		c.results[i].lock.Unlock()
	}
	return
}

func avg(ackTimes map[uint8]time.Duration) time.Duration {
	if len(ackTimes) <= 0 {
		return time.Duration(0)
	}
	avg := 0

	for _, v := range ackTimes {
		avg += int(v)
	}

	avgFl := float64(avg) / float64(len(ackTimes))

	return time.Duration(avgFl)
}

func (c *Client) writerToApp(fi int) {
	for p := range c.results[fi].payload {
		_, err := c.results[fi].pipeWriter.Write(p.data)
		if err != nil {
			// TODO: notify client?
		}
	}
}

func (c *Client) bufferResults() {
	timeout := time.NewTimer(30 * time.Second) //TODO: Adjust timeout duration

	for {
		select {
		case p := <-c.payload:
			i := p.fileIndex

			if p.offset != c.results[i].pointer {
				c.results[i].buffer = append(c.results[i].buffer, *p)
				continue
			}

			c.results[i].pointer++
			c.results[i].payload <- p
			c.ackNum <- p.ackNumber
		case <-timeout.C:
			// TODO: Close connection. Maybe retry?
		}
	}
}

func (c *Client) handleMetadata(_ io.Writer, p *packet) {
	smd := ServerMetaData{}
	err := smd.UnmarshalBinary(p.data)
	if err != nil {
		// TODO: what now? Rerequest metadata.
		// Maybe log something or cancel the whole thing?
	}
	c.smd <- &smd
	// TODO: Decide when to actually close smdChan
	close(c.smd)
}

func (c *Client) handleServerPayload(_ io.Writer, p *packet) {
	pl := ServerPayload{}
	err := pl.UnmarshalBinary(p.data)
	if err != nil {
		// TODO: what now? Rerequest payload
		// Maybe log something or cancel the whole thing?
	}
	c.payload <- &pl
}

func (c *Client) handleClose(_ io.Writer, p *packet) {
	cl := CloseConnection{}
	err := cl.UnmarshalBinary(p.data)
	if err != nil {
		// TODO: what now? Just drop everything?
	}
}
