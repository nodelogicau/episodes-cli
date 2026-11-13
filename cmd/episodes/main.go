// Command episodes is the reference implementation of the Episodes Format: a
// CLI for workspaces of episodic memory.
//
// Origin: nodelogicau/intentions-cli cmd/intentions/main.go @ b72a395454aa
// (golang.org/x/term replaced by a stat of stdin, to keep the dependencies to
// those the design names)
package main

import (
	"os"
	_ "time/tzdata" // the static binary carries the IANA database

	"github.com/nodelogicau/episodes-cli/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, func() bool {
		fi, err := os.Stdin.Stat()
		return err == nil && fi.Mode()&os.ModeCharDevice != 0
	}))
}
