// Package cmd implements command line handling
package cmd

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/hendrikcech/rft/rftp"
	"github.com/spf13/cobra"
	"math/rand"
	"time"
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

		if (p != -1 && (p < 0 || p > 1)) || (q != -1 && (q < 0 || p > 1)) {
			log.Print("p and q must be value between 0 and 1")
			os.Exit(1)
		} else if p == -1 && q != -1 {
			p = q
		} else if p != -1 && q == -1 {
			q = p
		} else if p == -1 && q == -1 {
			p = 0
			q = 0
		}

		rand.Seed(time.Now().UTC().UnixNano())
		lossSim := rftp.NewLossSimulator(p, q)
		conn := rftp.NewUdpConnection(lossSim)

		if s {
			log.Printf("running server on host '%v' and dir %v\n", host, files[0])
			server := rftp.NewServer(rftp.DirectoryLister(files[0]), conn)
			server.Listen(fmt.Sprintf(":%v", t))
			return
		}

		hs := fmt.Sprintf("%v:%v", host, t)
		log.Printf("running client request to host '%v' for files %v\n", hs, files)

		client := rftp.Client{}
		rs, err := client.Request(conn, hs, files)
		if err != nil {
			log.Printf("error on request: %v\n", err)
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
	rootCmd.PersistentFlags().Float32VarP(&p, "p", "p", -1, `specify the  loss probabilities for the Markov chain model (0 <= p <= 1)
if only one is specified, assume p=q; if neither is specified assume no loss`)
	rootCmd.PersistentFlags().Float32VarP(&q, "q", "q", -1, `specify the  loss probabilities for the Markov chain model (0 <= p <= 1)
if only one is specified, assume p=q; if neither is specified assume no loss`)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
