package main

import (
	"os"

	"github.com/cafaye/cafaye-cli/cmd"
)

func main() {
	if err := cmd.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
