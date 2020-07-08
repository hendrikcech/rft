package rftp

import (
	"encoding"
	"fmt"
	"io"
	"log"
	"strings"
)

var defaultClient = Client{}

func Request(host string, files []string) ([]Result, error) {
	return defaultClient.Request(NewUdpConnection(), host, files)
}

type Requester interface {
	Request(string, encoding.BinaryMarshaler) (encoding.BinaryUnmarshaler, error)
}

type Result struct {
	buffer     []ServerPayload
	pipeReader io.Reader
	pipeWriter io.Writer
	payload    chan *ServerPayload
	pointer    uint64
	size       uint64
	checksum   [16]byte
	err        error
}

func (r *Result) Read(p []byte) (n int, err error) {
	return r.pipeReader.Read(p)
}

type Client struct {
	results []Result
	smd     chan *ServerMetaData
	payload chan *ServerPayload
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

	conn.handle(msgServerMetadata, handlerFunc(c.handleMetadata))
	conn.handle(msgServerPayload, handlerFunc(c.handleServerPayload))
	conn.handle(msgClose, handlerFunc(c.handleClose))

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

	var errStrings []string
	// TODO: Decide when to close smdChan
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

func (c *Client) writerToApp(fi int) {
	for p := range c.results[fi].payload {
		_, err := c.results[p.fileIndex].pipeWriter.Write(p.data)
		if err != nil {
			// TODO: notify client?
		}
	}
}

func (c *Client) bufferResults() {
	for {
		select {
		case p := <-c.payload:
			i := p.fileIndex

			log.Printf("expecting packet: %v\n", c.results[i].pointer)
			log.Printf("received payload packet: %v\n", p)
			if p.offset != c.results[i].pointer {
				c.results[i].buffer = append(c.results[i].buffer, *p)
				continue
			}

			c.results[i].pointer++
			c.results[i].payload <- p
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
