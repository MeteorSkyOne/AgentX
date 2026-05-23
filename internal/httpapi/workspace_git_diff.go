package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

var (
	errWorkspaceGitBinary   = errors.New("workspace git preview is binary")
	errWorkspaceGitTooLarge = errors.New("workspace git preview is too large")
)

func workspaceGitDiffContents(ctx context.Context, gitCtx workspaceGitContext, scope string, baseRef string, compareRef string, change workspaceGitChange) (string, string, error) {
	switch scope {
	case workspaceGitScopeBranch, workspaceGitScopeCommit:
		original := ""
		modified := ""
		var err error
		if baseRef != "" {
			original, _, err = workspaceGitTextAtRefOptional(ctx, gitCtx.repoRoot, baseRef, workspaceGitOriginalRepoPath(change))
			if err != nil {
				return "", "", err
			}
		}
		if change.Status != "deleted" {
			modified, _, err = workspaceGitTextAtRefOptional(ctx, gitCtx.repoRoot, compareRef, change.repoPath)
			if err != nil {
				return "", "", err
			}
		}
		return original, modified, nil
	default:
		original := ""
		modified := ""
		var err error
		if change.Status != "added" && change.Status != "untracked" {
			original, err = workspaceGitTextAtRef(ctx, gitCtx.repoRoot, "HEAD", workspaceGitOriginalRepoPath(change))
			if err != nil {
				return "", "", err
			}
		}
		if change.Status != "deleted" {
			modified, err = workspaceGitTextFromWorkspace(gitCtx.workspaceRoot, change.Path)
			if err != nil {
				return "", "", err
			}
		}
		return original, modified, nil
	}
}

func workspaceGitOriginalRepoPath(change workspaceGitChange) string {
	if change.oldRepoPath != "" {
		return change.oldRepoPath
	}
	return change.repoPath
}

func workspaceGitTextAtRef(ctx context.Context, repoRoot string, ref string, repoPath string) (string, error) {
	output, err := workspaceGitBytes(ctx, repoRoot, "show", fmt.Sprintf("%s:%s", ref, repoPath))
	if err != nil {
		return "", err
	}
	return workspaceGitPreviewText(output)
}

func workspaceGitTextAtRefOptional(ctx context.Context, repoRoot string, ref string, repoPath string) (string, bool, error) {
	if repoPath == "" {
		return "", false, nil
	}
	revision := fmt.Sprintf("%s:%s", ref, repoPath)
	if err := workspaceGitRun(ctx, repoRoot, "cat-file", "-e", revision); err != nil {
		return "", false, nil
	}
	text, err := workspaceGitTextAtRef(ctx, repoRoot, ref, repoPath)
	if err != nil {
		return "", false, err
	}
	return text, true, nil
}

func workspaceGitTextFromWorkspace(workspaceRoot string, relPath string) (string, error) {
	target, err := safeWorkspacePath(workspaceRoot, relPath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", err
	}
	return workspaceGitPreviewText(data)
}

func workspaceGitPreviewText(data []byte) (string, error) {
	if len(data) > workspaceGitMaxPreviewBytes {
		return "", errWorkspaceGitTooLarge
	}
	if bytes.Contains(data, []byte{0}) || !utf8.Valid(data) {
		return "", errWorkspaceGitBinary
	}
	return workspaceGitNormalizePreviewText(string(data)), nil
}

func workspaceGitNormalizePreviewText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}
