package httpapi

import (
	"path/filepath"
	"strings"
)

func workspaceGitRepoPath(gitCtx workspaceGitContext, relPath string) string {
	relPath = filepath.ToSlash(strings.Trim(relPath, "/"))
	if gitCtx.prefix == "" {
		return relPath
	}
	if relPath == "" {
		return gitCtx.prefix
	}
	return strings.Trim(gitCtx.prefix, "/") + "/" + relPath
}

func appendWorkspaceGitPathspec(args []string, prefix string) []string {
	args = append(args, "--")
	if prefix != "" {
		args = append(args, prefix)
	}
	return args
}

func workspaceGitWorkspaceRelPath(gitCtx workspaceGitContext, repoPath string) (string, bool) {
	repoPath = filepath.ToSlash(strings.Trim(repoPath, "/"))
	prefix := strings.Trim(gitCtx.prefix, "/")
	if prefix == "" {
		return repoPath, repoPath != ""
	}
	if repoPath == prefix {
		return "", true
	}
	if !strings.HasPrefix(repoPath, prefix+"/") {
		return "", false
	}
	return strings.TrimPrefix(repoPath, prefix+"/"), true
}

func splitNUL(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, "\x00")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
