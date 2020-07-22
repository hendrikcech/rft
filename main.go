//+build !integration

package main

import (
	"github.com/hendrikcech/rft/cmd"
)

func main() {
	// uncomment to disable logging
	//log.SetOutput(ioutil.Discard)
	cmd.Execute()
}
