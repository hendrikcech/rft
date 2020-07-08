// Package cmd implements command line handling
package cmd

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/hendrikcech/rft/rftp"
	"github.com/spf13/cobra"
)

var (
	s    bool
	t    int
	p, q float32
)

var rootCmd = &cobra.Command{
	Use:   "rft <host> <file>",
	Short: "A sample client and server using rft",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		host := args[0]
		files := args[1:]

		if s {
			fmt.Printf("running server on host '%v' and dir %v\n", host, files[0])
			server := rftp.NewServer(rftp.DirectoryLister(files[0]), rftp.NewUdpConnection())
			server.Listen(fmt.Sprintf(":%v", t))
			return
		}

		hs := fmt.Sprintf("%v:%v", host, t)
		fmt.Printf("running client request to host '%v' for files %v\n", hs, files)
		rs, err := rftp.Request(hs, files)
		if err != nil {
			log.Printf("error on request: %v", err)
		}

		for _, r := range rs {
			io.Copy(os.Stdout, &r)
		}

	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&s, "server", "s", false, `server mode: accept incoming files from any host
Operate in client mode if “–s” is not specified`)
	rootCmd.PersistentFlags().IntVarP(&t, "port", "t", 0, "specify the port number to use")
	rootCmd.PersistentFlags().Float32VarP(&p, "p", "p", 0, `specify the  loss probabilities for the Markov chain model
if only one is specified, assume p=q; if neither is specified assume no loss`)
	rootCmd.PersistentFlags().Float32VarP(&q, "q", "q", 0, `specify the  loss probabilities for the Markov chain model
if only one is specified, assume p=q; if neither is specified assume no loss`)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
