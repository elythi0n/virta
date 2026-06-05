// Command virtad is the Virta engine daemon: it owns all platform connections, message
// normalization, storage, and the local API that every frontend (desktop, terminal, web,
// overlay) connects to as a client.
//
// For now it only reports its build version; the daemon's runtime — a loopback HTTP/
// WebSocket listener, a discovery file announcing its port and auth token, the message
// pipeline, and the API — is built out incrementally.
package main

import (
	"fmt"

	"github.com/elythi0n/virta/internal/buildinfo"
)

func main() {
	fmt.Printf("virtad %s\n", buildinfo.String())
	// TODO: start the loopback listener, write the discovery file, run the message
	// pipeline and sinks, and serve the HTTP/WebSocket API.
}
