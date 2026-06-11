package main

import (
	"net"
	"os"
	"strings"
)

// parseCLIDeepLink returns the first virta:// argument in args, or "".
func parseCLIDeepLink(args []string) string {
	for _, a := range args[1:] {
		if strings.HasPrefix(a, "virta://") {
			return a
		}
	}
	return ""
}

// forwardDeepLink tries to send a URL to an already-running instance via a Unix socket.
// Returns true if the URL was delivered (the caller should exit).
func forwardDeepLink(socketPath, url string) bool {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	_, _ = conn.Write([]byte(url))
	return true
}

// listenDeepLinks binds a Unix socket at socketPath and calls onURL for each
// URL forwarded by a new instance. Returns a close function and any bind error.
func listenDeepLinks(socketPath string, onURL func(string)) (func(), error) {
	_ = os.Remove(socketPath) // remove stale socket from a previous run
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	go func() {
		buf := make([]byte, 4096)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			n, _ := conn.Read(buf)
			_ = conn.Close()
			if n > 0 {
				if raw := strings.TrimSpace(string(buf[:n])); raw != "" {
					onURL(raw)
				}
			}
		}
	}()
	return func() { _ = ln.Close(); _ = os.Remove(socketPath) }, nil
}
