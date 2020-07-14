package rftp

import (
	"container/heap"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"strings"
	"sync"
	"time"
)

type Result struct {
	lock       sync.Mutex
	buffer     *chunkQueue
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
	payload    chan *ServerPayload
	pointer    uint64 // Next byte position to write out
	offset     uint64 // Offset as defined by rft
	done       bool   // true if error or all chunks written
	size       uint64
	checksum   [16]byte
	err        error
}

// TODO: After replacing buffer by a useful datastructure, calculate the
// complement of the buffered elements
func (r *Result) getResendEntries() []*ResendEntry {
	log.Printf("get resend entries from %v\n", r.pointer)
	return r.buffer.Gaps(r.pointer)
}

func (r *Result) Read(p []byte) (n int, err error) {
	return r.pipeReader.Read(p)
}

type Client struct {
	conn    connection
	results []Result
	rttLock sync.Mutex
	rtt     time.Duration
	smd     chan *ServerMetaData
	payload chan *ServerPayload
	ackNum  chan uint8

	timeout         *time.Timer
	timeoutCanceler chan struct{}
}

func (c *Client) Request(conn connection, host string, files []string) ([]Result, error) {
	if len(files) > 65536 {
		return nil, errors.New("too many files in request, use max. 65536 files per request")
	}
	fs := []FileDescriptor{}
	for i, f := range files {
		fs = append(fs, FileDescriptor{0, f})
		r, w := io.Pipe()
		c.results = append(c.results, Result{
			buffer:     newChunkQueue(uint16(i)),
			pipeReader: r,
			pipeWriter: w,
			payload:    make(chan *ServerPayload, 10),
		})
		go c.writerToApp(i)
	}

	c.conn = conn
	c.smd = make(chan *ServerMetaData, len(fs))
	c.payload = make(chan *ServerPayload, 1024*len(fs))
	c.ackNum = make(chan uint8, 256)
	c.timeoutCanceler = make(chan struct{}, 1)
	c.rtt = 1 * time.Second // TODO: set better initial timeout value
	c.timeout = time.NewTimer(6 * c.rtt)

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
	go c.timeoutConnection()

	var errStrings []string
	for smd := range c.smd {
		i := smd.fileIndex

		c.results[i].lock.Lock()

		if smd.status != 0 {
			err := fmt.Errorf("error receiving file: %v", smd.status)
			errStrings = append(errStrings, err.Error())
			c.results[i].err = err
			c.results[i].done = true
			close(c.results[i].payload)
			continue
		}

		c.results[i].size = smd.size
		c.results[i].checksum = smd.checkSum

		c.results[i].lock.Unlock()
	}

	defer func() {
		go c.closeConnection()
	}()

	if len(errStrings) > 0 {
		return c.results, fmt.Errorf(strings.Join(errStrings, ", "))
	}
	return c.results, nil
}

func (c *Client) ackNumHandler(hf handlerFunc) handlerFunc {
	return func(w io.Writer, p *packet) {
		c.ackNum <- p.ackNum
		c.timeoutCanceler <- struct{}{}
		rtt := c.getRTT()
		c.timeout = time.NewTimer(6 * rtt)
		go c.timeoutConnection()
		hf(w, p)
	}
}

func (c *Client) timeoutConnection() {
	select {
	case <-c.timeout.C:
		c.closeConnection()
	case <-c.timeoutCanceler:
		return
	}
}

func (c *Client) closeConnection() {
	c.timeoutCanceler <- struct{}{}
	c.conn.cclose(time.NewTimer(10 * time.Second))
	close(c.payload)
	//close(c.smd)
	log.Println("closing client")
}

func (c *Client) getRTT() time.Duration {
	c.rttLock.Lock()
	defer c.rttLock.Unlock()
	return c.rtt
}

func (c *Client) setRTT(rtt time.Duration) {
	c.rttLock.Lock()
	defer c.rttLock.Unlock()
	c.rtt = rtt
}

func (c *Client) sendAcks(conn connection) {

	nextAckNumber := uint8(1) // need to start with 1, to be able to distinguish between server header with no ACK number and our first ACK number

	ackSendMap := map[uint8]time.Time{}
	rttMap := map[uint8]time.Duration{}

	timeout := time.NewTimer(200 * time.Millisecond) // TODO: use better timeout before first acknum is received
	for {
		select {
		case an, more := <-c.ackNum:
			if !more {
				log.Println("closing ack sender")
				return
			}
			if an <= 0 {
				continue
			}
			rttMap[an] = time.Since(ackSendMap[an])
			c.rtt = time.Since(ackSendMap[an])
			log.Printf("received acknum echo: acknum: %v, time.Since = %v\n", an, c.rtt)

		case <-timeout.C:
			res, fi, off := c.getAckData()

			ack := ClientAck{
				ackNumber:           nextAckNumber,
				maxTransmissionRate: 0,
				fileIndex:           fi,
				offset:              off,
				resendEntries:       res,
			}
			log.Printf("sending ack: num: %v, off: %v, res: %v\n", ack.ackNumber, ack.offset, ack.resendEntries)
			conn.send(ack)

			log.Printf("set ack timer to: %v\n", c.rtt)
			timeout = time.NewTimer(c.rtt)
		}
	}
}

func (c *Client) getAckData() (res []*ResendEntry, fi uint16, off uint64) {
	for i := 0; i < len(c.results); i++ {
		c.results[i].lock.Lock()
		if c.results[i].offset > 0 {
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
	log.Printf("averaging map: %v\n", ackTimes)
	avg := 0

	for _, v := range ackTimes {
		avg += int(v)
	}

	avgFl := float64(avg) / float64(len(ackTimes))

	log.Printf("result float: %v\n", avgFl)
	d := time.Duration(math.Floor(avgFl))
	log.Printf("result duration: %v\n", d)
	return d
}

func (c *Client) writerToApp(fi int) {
	for p := range c.results[fi].payload {
		log.Printf("writing to app at offset: %v\n", p.offset)
		_, err := c.results[fi].pipeWriter.Write(p.data)
		// TODO: Finish up result, set done to true?
		if err != nil {
			// TODO: notify client?
		}
	}
	err := c.results[fi].pipeWriter.Close()
	log.Printf("Closing app writer with err: %v\n", err)
}

func (c *Client) bufferResults() {
	for p := range c.payload {
		log.Printf("received payload to buffer: %v\n", p.offset)
		i := p.fileIndex
		c.ackNum <- p.ackNumber

		c.results[i].lock.Lock()
		c.results[i].offset = p.offset
		if p.offset != c.results[i].pointer {
			heap.Push(c.results[i].buffer, p)
			c.results[i].lock.Unlock()
			continue
		}

		c.results[i].pointer++

		c.results[i].lock.Unlock()

		c.results[i].payload <- p

		c.results[i].lock.Lock()

		if c.results[i].buffer.Len() > 0 {
			top := c.results[i].buffer.Top()
			log.Printf("buffer top: %v\n", top)
			for top == c.results[i].pointer {
				p := heap.Pop(c.results[i].buffer).(*ServerPayload)
				c.results[i].pointer++

				c.results[i].lock.Unlock()
				c.results[i].payload <- p
				c.results[i].lock.Lock()
			}
		}

		c.results[i].lock.Unlock()
	}
	for i := 0; i < len(c.results); i++ {
		c.results[i].lock.Lock()
		if !c.results[i].done {
			c.results[i].done = true

			for c.results[i].buffer.Len() > 0 {
				p := heap.Pop(c.results[i].buffer).(*ServerPayload)
				if c.results[i].pointer != p.offset {
					log.Printf("Warning, possible packet loss: writing payload with offset %v at offset %v", p.offset, c.results[i].pointer)
				}
				c.results[i].payload <- p
				c.results[i].pointer++
			}

			close(c.results[i].payload)
		}
		c.results[i].lock.Unlock()
	}
	close(c.ackNum)
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
