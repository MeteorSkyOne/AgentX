//go:build !unix

package app

import "os/exec"

func configureProviderProbeCommand(cmd *exec.Cmd) {
}
