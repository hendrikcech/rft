package cmd

import (
	"fmt"
	"os"

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
	Run: func(cmd *cobra.Command, args []string) {
		if s {
			fmt.Println("running server")
			printFlags()
			return
		}
		fmt.Println("running client")
		printFlags()
	},
}

func printFlags() {
	fmt.Printf("t=%v\n", t)
	fmt.Printf("s=%v\n", s)
	fmt.Printf("p=%v\n", p)
	fmt.Printf("q=%v\n", q)
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&s, "server", "s", false, `server mode: accept incoming files from any host
Operate in client mode if “–s” is not specified`)
	rootCmd.PersistentFlags().IntVarP(&t, "port", "t", 0, "specify the port number to use")
	rootCmd.PersistentFlags().Float32VarP(&p, "p", "p", 0, `specify the  loss probabilities for the Markov chain model
if only one is specified, assume p=q; if neither is specified assume no loss`)
	rootCmd.PersistentFlags().Float32VarP(&p, "q", "q", 0, `specify the  loss probabilities for the Markov chain model
if only one is specified, assume p=q; if neither is specified assume no loss`)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
