package cmd

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

type testfile struct {
	size int64
	name string

	timeout time.Duration
}

func (t testfile) String() string {
	return fmt.Sprintf("size: %v, timeout: %v\n", t.size, t.timeout)
}

type tests []testfile

func (t tests) String() string {
	res := ""
	for i, tt := range t {
		res += fmt.Sprintf("\t%v: %v", i+1, tt)
	}
	return res
}

var testfiles = tests([]testfile{
	{size: 0, timeout: 5 * time.Second},
	{size: 10, timeout: 5 * time.Second},
	{size: 1 * 1000, timeout: 5 * time.Second},
	{size: 100 * 1024, timeout: 20 * time.Second},
	{size: 1000 * 1024, timeout: 1 * time.Minute},
})

type runner struct {
	src  string
	dest string

	tf []testfile
}

func (r *runner) setup() error {
	src, err := ioutil.TempDir("", "rftpSrc")
	if err != nil {
		return err
	}

	r.src = src

	dest, err := ioutil.TempDir("", "rftpDest")
	if err != nil {
		return err
	}

	r.dest = dest

	for _, tf := range testfiles {
		file, err := ioutil.TempFile(r.src, fmt.Sprintf("%v-", tf.size))
		if err != nil {
			return err
		}
		// write random content
		_, err = io.CopyN(file, rand.Reader, tf.size)
		if err != nil {
			return err
		}

		fi, err := file.Stat()
		if err != nil {
			return err
		}
		r.tf = append(r.tf, testfile{size: fi.Size(), name: fi.Name(), timeout: tf.timeout})

		if err := file.Close(); err != nil {
			return err
		}
	}

	return nil
}

func (r *runner) cleanup() (err error) {
	srcErr := os.RemoveAll(r.src)
	if srcErr != nil {
		err = fmt.Errorf("%v", srcErr)
	}
	destErr := os.RemoveAll(r.dest)
	if destErr != nil {
		err = fmt.Errorf("%v, %v", err, destErr)
	}

	return
}

type combination struct {
	server, client []string
}

func getServerClientCombinations(binaries []string, p, q float32) []combination {
	cc := []combination{}

	for _, bs := range binaries {
		for _, bc := range binaries {
			c := combination{
				server: []string{bs, "-s", "-q", fmt.Sprintf("%f", q), "-p", fmt.Sprintf("%f", p), "-t", "8080", "0.0.0.0"},
				client: []string{bc, "localhost", "-q", fmt.Sprintf("%f", q), "-p", fmt.Sprintf("%f", p), "-t", "8080"},
			}

			cc = append(cc, c)
		}
	}

	return cc
}

var benchCmd = &cobra.Command{
	Use:   "bench <rft1> <rft2>",
	Short: "An automatic benchmark of rft implementations",
	Long: fmt.Sprintf(`bench runs all combinations in which <rft1> and <rft2> can be used to
download files from each other. Use the -r flag, to only run a
specific combination. The order is deterministic:

	1: <rft1-server><rft1-client>
	2: <rft1-server><rft2-client>
	3: <rft2-server><rft1-client>
	4: <rft2-server><rft2-client>

Use the -s flag to only run tests of a certain file, currently configured are:

%v
`, testfiles),
	Args: cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		log.Println(args)

		binary1Path, err := filepath.Abs(args[0])
		if err != nil {
			log.Fatalf("failed to set rft path: %v\n", err)
		}
		binary2Path, err := filepath.Abs(args[1])
		if err != nil {
			log.Fatalf("failed to set rft path: %v\n", err)
		}
		cc := getServerClientCombinations([]string{binary1Path, binary2Path}, p, q)

		for r, c := range cc {
			if run != 0 && run != r+1 {
				continue
			}

			r := runner{}
			err = r.setup()
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("setup directories: src: %v, dest: %v\n", r.src, r.dest)

			serverCMD := exec.Command(c.server[0], append(c.server[1:], r.src)...)
			serverCMD.Dir = r.src
			log.Printf("run server: %v\n", serverCMD.Args)

			//serverCMD.Stdout = os.Stdout
			//serverCMD.Stderr = os.Stderr

			err = serverCMD.Start()
			if err != nil {
				log.Fatalf("failed to create run test server: %v\n", err)
			}

			for i, tf := range r.tf {
				if size != 0 && size != i+1 {
					continue
				}
				clientCMD := exec.Command(c.client[0], append(c.client[1:], tf.name)...)
				clientCMD.Dir = r.dest
				if i == 0 {
					log.Printf("run client: %v\n", clientCMD.Args)
				}
				//clientCMD.Stdout = os.Stdout
				//clientCMD.Stderr = os.Stderr

				start := time.Now()
				err = clientCMD.Start()
				if err != nil {
					log.Printf("failed to download file: %v\n", err)
				}

				done := make(chan error, 1)
				go func() {
					done <- clientCMD.Wait()
				}()

				select {
				case <-time.After(tf.timeout):
					if err := clientCMD.Process.Kill(); err != nil {
						log.Printf("failed to kill client process: %v", err)
					} else {
						log.Printf("killed client process after timeout: %v", tf.timeout)
					}
				case err := <-done:
					if err != nil {
						log.Printf("client crashed: %v", err)
					}
				}

				duration := time.Since(start)

				if compareFiles(filepath.Join(r.src, tf.name), filepath.Join(r.dest, tf.name)) {
					log.Printf("succesfully transferred file of size %v bytes in %v\n", tf.size, duration)
				} else {
					log.Printf("incorrectly transferred file of size %v bytes in %v\n", tf.size, duration)
				}
			}

			serverCMD.Process.Signal(syscall.SIGTERM)

			done := make(chan error, 1)
			go func() {
				done <- serverCMD.Wait()
			}()
			select {
			case <-time.After(1 * time.Second):
				if err := serverCMD.Process.Kill(); err != nil {
					log.Printf("failed to server kill process: %v", err)
				} else {
					log.Printf("killed server process after timeout")
				}
			case err := <-done:
				if err != nil {
					log.Printf("server crashed: %v\n", err)
				}
			}

			err := r.cleanup()
			if err != nil {
				log.Fatal(err)
			}

			time.Sleep(1 * time.Second)
			fmt.Println()
		}
	},
}

var run int
var size int

func init() {
	benchCmd.Flags().IntVarP(&run, "run", "r", 0, "Specify which run should be executed, 0 runs all combinations")
	benchCmd.Flags().IntVarP(&size, "size", "s", 0, "Specify which file size should be tested, 0 runs all configured sizes")
	rootCmd.AddCommand(benchCmd)
}

const chunkSize = 64000

func compareFiles(a, b string) bool {
	f1, err := os.Open(a)
	if err != nil {
		log.Println(err)
		return false
	}
	defer f1.Close()

	f2, err := os.Open(b)
	if err != nil {
		log.Println(err)
		return false
	}
	defer f2.Close()

	for {
		b1 := make([]byte, chunkSize)
		_, err1 := f1.Read(b1)

		b2 := make([]byte, chunkSize)
		_, err2 := f2.Read(b2)

		if err1 != nil || err2 != nil {
			if err1 == io.EOF && err2 == io.EOF {
				return true
			} else if err1 == io.EOF || err2 == io.EOF {
				return false
			} else {
				log.Println(err1, err2)
			}
		}

		if !bytes.Equal(b1, b2) {
			return false
		}
	}
}
