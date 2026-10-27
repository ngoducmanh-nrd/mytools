package main

import (
	"fmt"
	"os"

	"mytools/internal/ui/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
