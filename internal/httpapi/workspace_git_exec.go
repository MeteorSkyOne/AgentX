package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func workspaceGitRun(ctx context.Context, dir string, args ...string) error {
	_, err := workspaceGitBytes(ctx, dir, args...)
	return err
}

func workspaceGitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	output, err := workspaceGitBytes(ctx, dir, args...)
	return string(output), err
}

func workspaceGitBytes(ctx context.Context, dir string, args ...string) ([]byte, error) {
	gitCtx, cancel := context.WithTimeout(ctx, workspaceGitCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(gitCtx, "git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return nil, fmt.Errorf("git %s failed: %s: %w", args[0], detail, err)
		}
		return nil, err
	}
	return output, nil
}
