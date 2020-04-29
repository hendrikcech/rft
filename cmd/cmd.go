package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "rft",
	Short: "A sample client and server using rft",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("hellow world")
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
