// Command tokengen renders the design-system token source (frontends/ui-kit/tokens.json) into
// its generated artifacts (tokens.css, tokens.ts) in the same directory. Run via `make tokens`
// after editing tokens.json.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elythi0n/virta/internal/uikit"
)

func main() {
	dir := flag.String("dir", "frontends/ui-kit", "directory holding tokens.json (outputs are written here)")
	flag.Parse()

	src := filepath.Join(*dir, "tokens.json")
	b, err := os.ReadFile(src)
	if err != nil {
		fatal(err)
	}
	t, err := uikit.Load(b)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*dir, "tokens.css"), []byte(t.CSS()), 0o644); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*dir, "tokens.ts"), []byte(t.TS()), 0o644); err != nil {
		fatal(err)
	}
	fmt.Println("✓ generated tokens.css + tokens.ts from", src)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "tokengen:", err)
	os.Exit(1)
}
