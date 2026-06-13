//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW prevents Windows from allocating a console for the child process.
// https://learn.microsoft.com/windows/win32/procthread/process-creation-flags
const createNoWindow = 0x08000000

// hideDaemonWindow ensures the spawned virtad.exe runs without flashing a console window.
func hideDaemonWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
