package rftp

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"time"
)

var defaultClient = Client{
	Conn: NewUDPConnection(),
}

func Request(host string, files []string) ([]*FileResponse, error) {
	return defaultClient.Request(host, files)
}

type Client struct {
	Conn connection
	rtt  time.Duration

	responses []*FileResponse
	ack       chan uint8
	err       chan struct{}
	closeMsg  chan struct{}
	done      chan uint16
	stopAck   chan struct{}
	start     time.Time
}

func (c *Client) Request(host string, files []string) ([]*FileResponse, error) {

	if len(files) > 65536 {
		return nil, errors.New("too many files in request, use max. 65536 files per request")
	}

	fs := make([]FileDescriptor, len(files))
	c.responses = make([]*FileResponse, len(files))
	c.ack = make(chan uint8, 1024)
	c.err = make(chan struct{})
	c.closeMsg = make(chan struct{})
	c.done = make(chan uint16, len(fs))
	c.stopAck = make(chan struct{})

	for i, f := range files {
		fs[i] = FileDescriptor{0, f}
		c.responses[i] = newFileResponse(f, uint16(i))
		go c.responses[i].write(c.done)
	}

	c.Conn.handle(msgServerMetadata, handlerFunc(c.handleMetadata))
	c.Conn.handle(msgServerPayload, handlerFunc(c.handleServerPayload))
	c.Conn.handle(msgClose, handlerFunc(c.handleClose))

	if err := c.sendRequest(host, fs); err != nil {
		return nil, err
	}

	return c.responses, nil
}

func (c *Client) sendRequest(host string, fs []FileDescriptor) error {
	for i := 1; i <= 10; i++ {
		if err := c.Conn.connectTo(host); err != nil {
			return err
		}
		c.start = time.Now()
		if err := c.Conn.send(ClientRequest{
			maxTransmissionRate: 0,
			files:               fs,
		}); err != nil {
			return err
		}

		go func() {
			err := c.Conn.receive()
			if err != nil {
				log.Println("receive crashed with err")
				c.err <- struct{}{}
			}
		}()
		if err := c.waitForFirstResponse(i); err != nil {
			log.Printf("err: %v, try again\n", err)
			c.Conn.cclose(0 * time.Second)
			continue
		}

		go c.sendAcks(c.Conn)
		go c.waitForCloseConnection()
		return nil
	}

	return fmt.Errorf("request timed out %v times, aborting", 10)
}

func (c *Client) waitForCloseConnection() {
	done := 0
	for {
		select {
		case i := <-c.done:
			fr := c.responses[i]
			if fr.Err != nil {
				log.Printf("Transfer of file %v aborted: %s", i, fr.Err)
			}
			done++
			if done == len(c.responses) {
				c.closeConnection()
			}

		case <-c.closeMsg:
			c.closeConnection()
		case <-c.err:
			c.closeConnection()
		}
	}
}

func (c *Client) closeConnection() {
	c.stopAck <- struct{}{}
	for _, r := range c.responses {
		log.Printf("send abort to file writer: %v\n", r.index)
		r.cc <- struct{}{}
	}
	c.Conn.cclose(1 * time.Second)
}

func (c *Client) waitForFirstResponse(try int) error {
	exp := math.Pow(2, float64(try))
	timeoutTime := time.Duration(exp) * time.Second // TODO Set initial timeout with expo backoff
	timeout := time.NewTimer(timeoutTime)
	select {
	case <-timeout.C:
		return fmt.Errorf("%v. try timed out after %v", try, timeoutTime)
	case <-c.ack:
		c.rtt = time.Since(c.start)
		return nil
	}
}

func (c *Client) sendAcks(conn connection) {
	timeout := time.NewTimer(1 * time.Millisecond)
	ackNumWaitingMap := map[uint8]bool{}
	ackSendTimeMap := map[uint8]time.Time{}
	nextAckNum := uint8(1)
	lastPing := time.Now()

	for {
		select {
		case <-timeout.C:
			if time.Since(lastPing) > 1*time.Second+10000*c.rtt {
				log.Println("connection timed out")
				c.err <- struct{}{}
				continue
			}
			maxFile := uint16(0)
			maxOff := uint64(0)
			metaDataMissing := false
			maxTransmission := 1
			res := []*ResendEntry{}
			for i, r := range c.responses {
				if len(res) > 3 {
					break
				}
				index := uint16(i)
				rd := r.getResendEntries(140)
				maxTransmission += rd.bufferSize
				if rd.res != nil {
					res = append(res, rd.res...)
				}
				if index == maxFile {
					maxOff = rd.head
				}
				if index > maxFile && rd.started {
					maxFile = index
					maxOff = rd.head
					if !rd.metadata {
						metaDataMissing = true
					}
				}
			}
			ack := ClientAck{
				ackNumber:           nextAckNum,
				maxTransmissionRate: uint32(maxTransmission),
				fileIndex:           maxFile,
				rtt:                 c.rtt.Milliseconds(),
				offset:              maxOff,
				resendEntries:       res,
				metaDataMissing:     metaDataMissing,
			}
			ackSendTimeMap[nextAckNum] = time.Now()
			ackNumWaitingMap[nextAckNum] = true
			log.Printf("sending ack at timeout: %v: %v\n", c.rtt, &ack)
			c.Conn.send(ack)

			nextAckNum++
			// avoid 0 as it can't be distinguished from not set
			if nextAckNum == 0 {
				nextAckNum++
			}
			//timeout = time.NewTimer(500 * time.Millisecond)
			timeout = time.NewTimer(c.rtt)

		case ackNum := <-c.ack:
			if waiting, ok := ackNumWaitingMap[ackNum]; ok && waiting {
				if sent, ok := ackSendTimeMap[ackNum]; ok {
					c.rtt = time.Since(sent)
					ackNumWaitingMap[ackNum] = false
					log.Printf("got new rtt: %v\n", c.rtt)
				}
			}
			lastPing = time.Now()

		case <-c.stopAck:
			log.Println("leaving ack writer")
			return
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
	c.ack <- p.ackNum
	log.Printf("handling metadata for file %v\n", smd.fileIndex)
	c.responses[smd.fileIndex].mc <- &smd
}

func (c *Client) handleServerPayload(_ io.Writer, p *packet) {
	pl := ServerPayload{}
	err := pl.UnmarshalBinary(p.data)
	if err != nil {
		// TODO: what now? Rerequest payload
		// Maybe log something or cancel the whole thing?
	}
	c.ack <- p.ackNum
	log.Printf("handling payload %v for file %v\n", pl.offset, pl.fileIndex)
	c.responses[pl.fileIndex].pc <- &pl
}

func (c *Client) handleClose(_ io.Writer, p *packet) {
	cl := CloseConnection{}
	err := cl.UnmarshalBinary(p.data)
	if err != nil {
		// TODO: what now? Just drop everything?
	}
	c.ack <- p.ackNum
	c.closeMsg <- struct{}{}
}
