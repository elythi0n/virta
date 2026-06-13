package main

import (
	"embed"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
)

// daemonBinary embeds the virtad executable for this OS/arch, placed by `make app` before the
// build. Embed-and-extract: one downloadable artifact carries the daemon, so there is
// nothing separate to install; we extract it and run it as a child process.
//
//go:embed all:bin
var daemonBinary embed.FS

type daemonProcess struct{ cmd *exec.Cmd }

func (d *daemonProcess) stop() {
	if d == nil || d.cmd == nil || d.cmd.Process == nil {
		return
	}
	// SIGTERM lets virtad shut down gracefully (it removes its discovery file). Windows ignores
	// the signal value and this falls back to a kill; refine per-OS when Windows packaging lands.
	_ = d.cmd.Process.Signal(syscall.SIGTERM)
}

// launchDaemon extracts the embedded virtad to the user cache dir and starts it. Called only when
// Attach found no daemon already running.
func (a *App) launchDaemon() error {
	path, err := extractDaemon()
	if err != nil {
		return err
	}
	cmd := exec.Command(path)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	// On Windows the shell is a GUI app with no console; spawning the console-subsystem virtad.exe
	// would otherwise pop a new console window. hideDaemonWindow sets CREATE_NO_WINDOW (no-op elsewhere).
	hideDaemonWindow(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	a.daemon = &daemonProcess{cmd: cmd}
	return nil
}

func extractDaemon() (string, error) {
	name := "virtad"
	if runtime.GOOS == "windows" {
		name = "virtad.exe"
	}
	data, err := daemonBinary.ReadFile("bin/" + name)
	if err != nil {
		return "", err
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cacheDir, "virta")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, name)
	if err := os.WriteFile(dest, data, 0o755); err != nil {
		return "", err
	}
	return dest, nil
}
