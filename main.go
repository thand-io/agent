package main

import (
	"fmt"
	"os"

	"github.com/thand-io/agent/cmd/cli"
)

func main() {
	if err := cli.GetCommandOptions().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
