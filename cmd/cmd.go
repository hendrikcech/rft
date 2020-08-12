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

	"path/filepath"

	"github.com/hendrikcech/rft/rftp"
	"github.com/spf13/cobra"
)

var (
	s    bool
	t    int
	p, q float32
	out  string
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
			log.Printf("start file server for dir %v\n", files[0])
			server := rftp.NewServer()
			if p != -1 || q != -1 {
				lossSim := rftp.NewMarkovLossSimulator(p, q)
				server.Conn.LossSim(lossSim)
				rand.Seed(time.Now().UTC().UnixNano())
			}
			dh, err := directoryHandler(files[0])
			if err != nil {
				log.Printf("Can not serve directory %s: %s", files[0], err)
				return
			}
			server.SetFileHandler(dh)
			err = server.Listen(fmt.Sprintf(":%v", t))
			if err != nil {
				log.Println(err)
			}
			return
		}

		// uncomment to disable logging
		log.SetOutput(ioutil.Discard)
		if info, err := os.Stat(out); out != "-" && (err != nil || !info.IsDir()) {
			log.Printf("Invalid out path")
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

		reqs, err := client.Request(hs, files)
		if err != nil {
			log.Printf("error on request: %v\n", err)
		}

		for i, req := range reqs {
			var w io.Writer
			if out == "-" {
				w = os.Stdout
			} else {
				path := filepath.Join(out, files[i])
				w, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
				if err != nil {
					log.Printf("Can't write file to %s: %s", path, err)
					return
				}
			}

			io.Copy(w, req)

			if req.Err != nil {
				log.Printf("File %s error: %s", files[i], req.Err)
			} else {
				log.Printf("File %s received (checksum is valid)", files[i])
			}
		}

		// Without this not all goroutines are finishing. For example,
		// waitForCloseConnection does not process the write to the done channel by
		// the last processed FileResponse.
		time.Sleep(1 * time.Millisecond)
	},
}

func directoryHandler(dirname string) (rftp.FileHandler, error) {
	info, err := os.Stat(dirname)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("Is file, not directory")
	}

	type file struct {
		path string
		info os.FileInfo
	}

	var files []file
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if !info.IsDir() {
			p, err := filepath.Rel(dirname, path)
			if err != nil {
				return err
			}
			files = append(files, file{p, info})
		}
		return nil
	}
	if err := filepath.Walk(dirname, walkFn); err != nil {
		return nil, err
	}

	return func(name string, offset uint64) *io.SectionReader {
		for _, f := range files {
			if f.path == name {
				file, err := os.Open(filepath.Join(dirname, f.path))
				if err != nil {
					log.Printf("%s", err)
					return nil
				}
				log.Printf("handling file: %v, offset: %v, size: %v\n", file, int64(offset), f.info.Size())
				return io.NewSectionReader(file, int64(offset), f.info.Size())
			}
		}
		return nil
	}, nil
}

func init() {
	rootCmd.Flags().BoolVarP(&s, "server", "s", false,
		`server mode: accept incoming files from any host. Operate in client mode if
“–s” is not specified.`)

	rootCmd.Flags().IntVarP(&t, "port", "t", 0, "specify the port number to use")

	rootCmd.PersistentFlags().Float32VarP(&p, "p", "p", -1,
		`specify the loss probabilities for the Markov chain model (0 <= p <= 1). If
only one is specified, assume p=q; if neither is specified assume no loss`)

	rootCmd.PersistentFlags().Float32VarP(&q, "q", "q", -1,
		`specify the loss probabilities for the Markov chain model (0 <= p <= 1). If
only one is specified, assume p=q; if neither is specified assume no loss`)

	rootCmd.Flags().StringVarP(&out, "out", "o", ".",
		`specify the directory in which the requested files are going to be stored;
set to '-' to redirect file content to stdout`)

	rootCmd.Flags().SortFlags = false
	rootCmd.PersistentFlags().SortFlags = false
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
