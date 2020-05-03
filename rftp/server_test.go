package rftp_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/hendrikcech/rft/rftp"
)

type files string

func (fl files) List() ([]os.FileInfo, error) {
	return ioutil.ReadDir(string(fl))
}

func TestServer(t *testing.T) {
	s := rftp.Server{
		SRC: files("."),
	}
	s.Listen("localhost:8080")
}
