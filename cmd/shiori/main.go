// Command shiori is the standalone read-only workplan CLI (stage B).
package main

import (
	"os"

	"github.com/hoshinoht/shiori/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr)) }
