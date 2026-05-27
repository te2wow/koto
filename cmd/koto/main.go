// Command koto runs AI coding agents through a YAML workflow and keeps looping
// until the quality gates are green.
package main

import (
	"fmt"
	"os"

	"github.com/te2wow/koto/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(cli.ExitCode(err))
	}
}
