//go:build windows

package main

import (
	"log/slog"
	"os"
	"os/exec"
)

func execRestart(exe string) {
	slog.Info("restarting agentx", "executable", exe)
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		slog.Error("restart: start new process", "error", err)
		return
	}
	os.Exit(0)
}
