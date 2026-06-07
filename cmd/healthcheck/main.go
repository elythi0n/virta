// Command healthcheck makes a single HTTP GET to the URL in argv[1] and exits 0
// on a 2xx response, 1 otherwise. Compiled as a static binary and copied into the
// distroless production image so Docker can run the virtad health check.
package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: healthcheck <url>")
		os.Exit(1)
	}
	resp, err := http.Get(os.Args[1]) //nolint:gosec
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "status %d\n", resp.StatusCode)
		os.Exit(1)
	}
}
