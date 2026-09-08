// Command narrate turns written text into a conversational spoken script and,
// optionally, audio via the macOS native speech facility.
package main

import (
	"os"

	"example.com/narrate/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
