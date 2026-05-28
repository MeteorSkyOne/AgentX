//go:build !windows

package main

import (
	"log/slog"
	"os"
	"syscall"
)

func execRestart(exe string) {
	slog.Info("restarting agentx", "executable", exe)
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
		slog.Error("restart exec failed", "error", err)
	}
}
