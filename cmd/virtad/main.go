// Command virtad is the Virta engine daemon: it owns all platform connections,
// normalization, storage, and the local API that every frontend is a client of.
//
// This is the step-0.1 skeleton — it only reports its build. The real daemon (localhost
// listener, discovery file, pipeline, /v1 API) lands in step 0.6.
package main

import (
	"fmt"

	"github.com/elythi0n/virta/internal/buildinfo"
)

func main() {
	fmt.Printf("virtad %s\n", buildinfo.String())
	// TODO(0.6): start the localhost listener, write the discovery file, run the
	// pipeline + sinks, and serve the /v1 WebSocket/REST API.
}
