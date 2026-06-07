package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func readDiscovery(runtimeDir string) (struct{ Addr, Token string }, error) {
	path := filepath.Join(runtimeDir, "daemon.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return struct{ Addr, Token string }{}, fmt.Errorf("no discovery file at %s: %w", path, err)
	}
	var d struct {
		Addr  string `json:"addr"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return struct{ Addr, Token string }{}, err
	}
	return struct{ Addr, Token string }{Addr: d.Addr, Token: d.Token}, nil
}
