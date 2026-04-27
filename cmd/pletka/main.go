// Command pletka is the Pletka HTTP server binary.
//
// At v0.1.0-dev this is a placeholder that prints version information so the
// scaffold can compile and goreleaser can build a binary. Real wiring is
// added as domain and HTTP layers are filled in.
package main

import (
	"fmt"
	"os"
)

// Build-time variables populated by goreleaser via -ldflags.
var (
	version = "0.1.0-dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v" || os.Args[1] == "version") {
		fmt.Printf("pletka %s (commit %s, built %s)\n", version, commit, date)
		return
	}
	fmt.Printf("pletka %s — schema-driven web platform\n", version)
	fmt.Println("nothing to serve yet; this is a v0.1.0-dev scaffold.")
}
