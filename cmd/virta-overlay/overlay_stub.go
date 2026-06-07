//go:build !linux && !windows && !darwin

package main

import "fmt"

func runOverlay(urlStr, title string, x, y, width, height int) error {
	return fmt.Errorf("virta-overlay: unsupported platform")
}
