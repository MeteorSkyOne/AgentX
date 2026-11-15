//go:build unix

package agy

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type childProcInfo struct {
	pid  int
	ppid int
	pgid int
	args string
}

// stopProcessBackgroundTask terminates a background shell command spawned by
// `agy acp` (which launches each `run_command` in its own process group with
// `bash -c <cmdLine>`). Matching both PPID == agyPID and the command line keeps
// long-lived sibling children such as MCP servers untouched.
func stopProcessBackgroundTask(agyPID int, cmdLine string) error {
	cmdLine = strings.TrimSpace(cmdLine)
	if agyPID <= 0 || cmdLine == "" {
		return errNoSuchBackgroundTask
	}
	children := listChildProcesses(agyPID)
	var targets []childProcInfo
	for _, c := range children {
		if c.pid <= 1 || c.pid == agyPID {
			continue
		}
		if matchesShellCommand(c.args, cmdLine) {
			targets = append(targets, c)
		}
	}
	if len(targets) == 0 {
		return errNoSuchBackgroundTask
	}

	for _, t := range targets {
		signalChildGroupOrPID(t, syscall.SIGTERM)
	}

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		alive := false
		for _, t := range targets {
			if isProcessAlive(t.pid) {
				alive = true
				break
			}
		}
		if !alive {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, t := range targets {
		signalChildGroupOrPID(t, syscall.SIGKILL)
	}
	return nil
}

func isProcessAlive(pid int) bool {
	if pid <= 1 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	if statBytes, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat")); err == nil {
		state, _, _, ok := parseProcStatStatePPIDAndPGID(statBytes)
		if ok && (state == "Z" || state == "X" || state == "x") {
			return false
		}
		return ok
	}
	if _, err := os.Stat("/proc"); err == nil {
		// /proc exists on this OS, so missing /proc/<pid>/stat means exited.
		return false
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	st := strings.TrimSpace(string(out))
	return st != "" && !strings.HasPrefix(st, "Z")
}

func signalChildGroupOrPID(c childProcInfo, sig syscall.Signal) {
	// Only signal the negative process group when the child is its own group
	// leader (PID == PGID), which is how `agy` starts `run_command` shells.
	if c.pgid > 1 && c.pgid == c.pid {
		_ = syscall.Kill(-c.pgid, sig)
		return
	}
	_ = syscall.Kill(c.pid, sig)
}

func listChildProcesses(parentPID int) []childProcInfo {
	if entries, err := os.ReadDir("/proc"); err == nil {
		var out []childProcInfo
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			pid, err := strconv.Atoi(entry.Name())
			if err != nil || pid <= 1 {
				continue
			}
			statBytes, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
			if err != nil {
				continue
			}
			state, ppid, pgid, ok := parseProcStatStatePPIDAndPGID(statBytes)
			if !ok || ppid != parentPID || state == "Z" || state == "X" || state == "x" {
				continue
			}
			cmdBytes, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
			if err != nil || len(cmdBytes) == 0 {
				continue
			}
			args := strings.TrimSpace(string(bytes.ReplaceAll(bytes.TrimRight(cmdBytes, "\x00"), []byte{0}, []byte{' '})))
			out = append(out, childProcInfo{pid: pid, ppid: ppid, pgid: pgid, args: args})
		}
		return out
	}

	// Fallback for non-Linux Unix systems (e.g. macOS).
	raw, err := exec.Command("ps", "-eo", "pid=,ppid=,pgid=,args=").Output()
	if err != nil {
		return nil
	}
	var out []childProcInfo
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		pgid, err3 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil || err3 != nil || ppid != parentPID {
			continue
		}
		idx := strings.Index(line, fields[3])
		args := fields[3]
		if idx >= 0 {
			args = strings.TrimSpace(line[idx:])
		}
		out = append(out, childProcInfo{pid: pid, ppid: ppid, pgid: pgid, args: args})
	}
	return out
}

func parseProcStatStatePPIDAndPGID(stat []byte) (state string, ppid int, pgid int, ok bool) {
	// Format: pid (comm) state ppid pgrp ... — comm can contain spaces or ')'.
	closeParen := bytes.LastIndexByte(stat, ')')
	if closeParen < 0 || closeParen+2 >= len(stat) {
		return "", 0, 0, false
	}
	fields := strings.Fields(string(stat[closeParen+2:]))
	if len(fields) < 3 {
		return "", 0, 0, false
	}
	p1, err1 := strconv.Atoi(fields[1])
	p2, err2 := strconv.Atoi(fields[2])
	if err1 != nil || err2 != nil {
		return "", 0, 0, false
	}
	return fields[0], p1, p2, true
}
