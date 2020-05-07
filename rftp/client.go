package rftp

import (
	"encoding"
	"io"
)

var defaultRequester = UDPRequester{}

var defaultClient = Client{
	&defaultRequester,
}

func Request(host string, files []string) ([]io.Reader, error) {
	return defaultClient.Request(host, files)
}

type Requester interface {
	Request(string, encoding.BinaryMarshaler) (encoding.BinaryUnmarshaler, error)
}

type Client struct {
	Transport Requester
}

func (c Client) Request(host string, files []string) ([]io.Reader, error) {
	fs := []FileRequest{}
	for _, f := range files {
		fs = append(fs, FileRequest{0, f})
	}

	if c.Transport == nil {
		c.Transport = &defaultRequester
	}
	cr := ClientRequest{fs}
	_, err := c.Transport.Request(host, cr)
	if err != nil {
		return nil, err
	}

	// TODO: convert r into a list of reader and return it

	return nil, nil
}
