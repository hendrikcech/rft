package rftp_test

import (
	"testing"

	"github.com/hendrikcech/rft/rftp"
)

func checkErr(t *testing.T, err error) {
	if err != nil {
		t.Error(err)
	}
}

func TestClient(t *testing.T) {
	c := &rftp.Client{}
	_, err := c.Request("localhost:8000", []string{"File"})
	checkErr(t, err)
}
