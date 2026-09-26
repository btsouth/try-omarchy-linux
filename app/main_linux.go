//go:build linux

package main

import (
	"fmt"
	"os"
)

// The Linux front end is not written yet. This entry point keeps the shared
// launcher code building and tested on Linux in the meantime.
func main() {
	fmt.Fprintf(os.Stderr, "%s for Linux is not available yet.\n", appTitle)
	os.Exit(2)
}
