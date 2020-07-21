// Package cmd implements command line handling
package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	"math/rand"
	"time"

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

		if (p != -1 && (p < 0 || p > 1)) || (q != -1 && (q < 0 || p > 1)) {
			log.Print("p and q must be value between 0 and 1")
			os.Exit(1)
		} else if p == -1 && q != -1 {
			p = q
		} else if p != -1 && q == -1 {
			q = p
		}

		if s {
			log.Printf("running server on host '%v' and dir %v\n", host, files[0])
			server := rftp.NewServer()
			if p != -1 || q != -1 {
				lossSim := rftp.NewMarkovLossSimulator(p, q)
				server.Conn.LossSim(lossSim)
				rand.Seed(time.Now().UTC().UnixNano())
			}
			server.SetFileHandler(directoryHandler(files[0]))
			server.Listen(fmt.Sprintf(":%v", t))
			return
		}

		hs := fmt.Sprintf("%v:%v", host, t)
		log.Printf("running client request to host '%v' for files %v\n", hs, files)

		var client rftp.Client
		if p != -1 || q != -1 {
			lossSim := rftp.NewMarkovLossSimulator(p, q)
			conn := rftp.NewUDPConnection()
			conn.LossSim(lossSim)
			client = rftp.Client{Conn: conn}
			rand.Seed(time.Now().UTC().UnixNano())
		} else {
			client = rftp.Client{Conn: rftp.NewUDPConnection()}
		}

		rs, err := client.Request(hs, files)
		if err != nil {
			log.Printf("error on request: %v\n", err)
		}

		for _, r := range rs {
			io.Copy(os.Stdout, r)
			log.Println("finish write")
		}

	},
}

func directoryHandler(dirname string) rftp.FileHandler {
	fi, err := ioutil.ReadDir(dirname)
	if err != nil {
		return nil
	}
	return func(name string, offset uint64) *io.SectionReader {
		for _, f := range fi {
			if f.Name() == name {
				file, err := os.Open(f.Name())
				if err != nil {
					return nil
				}
				return io.NewSectionReader(file, int64(offset), f.Size())
			}
		}
		return nil
	}
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
