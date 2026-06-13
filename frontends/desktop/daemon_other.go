//go:build !windows

package main

import "os/exec"

// hideDaemonWindow is a no-op outside Windows: only Windows allocates a console for child processes.
func hideDaemonWindow(_ *exec.Cmd) {}
