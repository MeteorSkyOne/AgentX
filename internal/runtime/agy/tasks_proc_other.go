//go:build !unix

package agy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type winChildProc struct {
	ProcessID   int    `json:"ProcessId"`
	CommandLine string `json:"CommandLine"`
}

func stopProcessBackgroundTask(agyPID int, cmdLine string) error {
	cmdLine = strings.TrimSpace(cmdLine)
	if agyPID <= 0 || cmdLine == "" {
		return errNoSuchBackgroundTask
	}
	query := fmt.Sprintf(
		"Get-CimInstance Win32_Process -Filter 'ParentProcessId = %d' | Select-Object ProcessId,CommandLine | ConvertTo-Json -Compress",
		agyPID,
	)
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", query).Output()
	if err != nil {
		return errNoSuchBackgroundTask
	}
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 {
		return errNoSuchBackgroundTask
	}

	var children []winChildProc
	if trimmed[0] == '[' {
		_ = json.Unmarshal(trimmed, &children)
	} else {
		var single winChildProc
		if json.Unmarshal(trimmed, &single) == nil && single.ProcessID > 0 {
			children = []winChildProc{single}
		}
	}

	var targetPIDs []int
	for _, c := range children {
		if c.ProcessID <= 0 || c.ProcessID == agyPID {
			continue
		}
		if matchesShellCommand(strings.TrimSpace(c.CommandLine), cmdLine) {
			targetPIDs = append(targetPIDs, c.ProcessID)
		}
	}
	if len(targetPIDs) == 0 {
		return errNoSuchBackgroundTask
	}

	for _, pid := range targetPIDs {
		if err := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run(); err != nil {
			if p, findErr := os.FindProcess(pid); findErr == nil && p != nil {
				_ = p.Kill()
			}
		}
	}
	return nil
}
