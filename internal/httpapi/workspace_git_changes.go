package httpapi

import (
	"context"
	"path/filepath"
	"strings"
)

func workspaceGitWorkingTreeChanges(ctx context.Context, gitCtx workspaceGitContext) ([]workspaceGitChange, error) {
	args := []string{"status", "--porcelain=v1", "-z"}
	args = appendWorkspaceGitPathspec(args, gitCtx.prefix)
	output, err := workspaceGitOutput(ctx, gitCtx.repoRoot, args...)
	if err != nil {
		return nil, err
	}
	parts := splitNUL(output)
	changes := make([]workspaceGitChange, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		if len(part) < 4 {
			continue
		}
		indexStatus := part[:1]
		workStatus := part[1:2]
		repoPath := filepath.ToSlash(part[3:])
		oldRepoPath := ""
		if indexStatus == "R" || indexStatus == "C" || workStatus == "R" || workStatus == "C" {
			if i+1 < len(parts) {
				oldRepoPath = filepath.ToSlash(parts[i+1])
				i++
			}
		}
		change, ok := workspaceGitChangeFromStatus(gitCtx, repoPath, oldRepoPath, indexStatus, workStatus)
		if ok {
			changes = append(changes, change)
		}
	}
	return changes, nil
}

func workspaceGitBranchChanges(ctx context.Context, gitCtx workspaceGitContext, baseRef string, compareRef string) ([]workspaceGitChange, error) {
	args := []string{"diff", "--name-status", "-z", "--find-renames", baseRef, compareRef}
	args = appendWorkspaceGitPathspec(args, gitCtx.prefix)
	output, err := workspaceGitOutput(ctx, gitCtx.repoRoot, args...)
	if err != nil {
		return nil, err
	}
	parts := splitNUL(output)
	changes := make([]workspaceGitChange, 0, len(parts)/2)
	for i := 0; i < len(parts); i++ {
		status := parts[i]
		if status == "" {
			continue
		}
		code := status[:1]
		oldRepoPath := ""
		repoPath := ""
		if code == "R" || code == "C" {
			if i+2 >= len(parts) {
				break
			}
			oldRepoPath = filepath.ToSlash(parts[i+1])
			repoPath = filepath.ToSlash(parts[i+2])
			i += 2
		} else {
			if i+1 >= len(parts) {
				break
			}
			repoPath = filepath.ToSlash(parts[i+1])
			i++
		}
		change, ok := workspaceGitChangeFromNameStatus(gitCtx, repoPath, oldRepoPath, code)
		if ok {
			changes = append(changes, change)
		}
	}
	return changes, nil
}

func workspaceGitCommitChanges(ctx context.Context, gitCtx workspaceGitContext, commit string, parent string) ([]workspaceGitChange, error) {
	args := []string{"diff-tree", "--no-commit-id", "--name-status", "-z", "--find-renames", "-r"}
	if parent == "" {
		args = append(args, "--root", commit)
	} else {
		args = append(args, parent, commit)
	}
	args = appendWorkspaceGitPathspec(args, gitCtx.prefix)
	output, err := workspaceGitOutput(ctx, gitCtx.repoRoot, args...)
	if err != nil {
		return nil, err
	}
	return workspaceGitChangesFromNameStatus(gitCtx, output), nil
}

func workspaceGitChangesFromNameStatus(gitCtx workspaceGitContext, output string) []workspaceGitChange {
	parts := splitNUL(output)
	changes := make([]workspaceGitChange, 0, len(parts)/2)
	for i := 0; i < len(parts); i++ {
		status := parts[i]
		if status == "" {
			continue
		}
		code := status[:1]
		oldRepoPath := ""
		repoPath := ""
		if code == "R" || code == "C" {
			if i+2 >= len(parts) {
				break
			}
			oldRepoPath = filepath.ToSlash(parts[i+1])
			repoPath = filepath.ToSlash(parts[i+2])
			i += 2
		} else {
			if i+1 >= len(parts) {
				break
			}
			repoPath = filepath.ToSlash(parts[i+1])
			i++
		}
		change, ok := workspaceGitChangeFromNameStatus(gitCtx, repoPath, oldRepoPath, code)
		if ok {
			changes = append(changes, change)
		}
	}
	return changes
}

func workspaceGitChangeFromStatus(gitCtx workspaceGitContext, repoPath string, oldRepoPath string, indexStatus string, workStatus string) (workspaceGitChange, bool) {
	path, ok := workspaceGitWorkspaceRelPath(gitCtx, repoPath)
	if !ok {
		return workspaceGitChange{}, false
	}
	oldPath := ""
	if oldRepoPath != "" {
		oldPath, _ = workspaceGitWorkspaceRelPath(gitCtx, oldRepoPath)
	}
	status := "modified"
	switch {
	case indexStatus == "?" && workStatus == "?":
		status = "untracked"
	case indexStatus == "R" || workStatus == "R":
		status = "renamed"
	case indexStatus == "C" || workStatus == "C":
		status = "copied"
	case indexStatus == "A" || workStatus == "A":
		status = "added"
	case indexStatus == "D" || workStatus == "D":
		status = "deleted"
	case indexStatus == "T" || workStatus == "T":
		status = "typechange"
	}
	return workspaceGitChange{
		Path:        path,
		OldPath:     oldPath,
		Status:      status,
		Staged:      indexStatus != " " && indexStatus != "?",
		Unstaged:    workStatus != " " && workStatus != "?",
		Untracked:   indexStatus == "?" && workStatus == "?",
		IndexStatus: strings.TrimSpace(indexStatus),
		WorkStatus:  strings.TrimSpace(workStatus),
		repoPath:    repoPath,
		oldRepoPath: oldRepoPath,
	}, true
}

func workspaceGitChangeFromNameStatus(gitCtx workspaceGitContext, repoPath string, oldRepoPath string, code string) (workspaceGitChange, bool) {
	path, ok := workspaceGitWorkspaceRelPath(gitCtx, repoPath)
	if !ok {
		return workspaceGitChange{}, false
	}
	oldPath := ""
	if oldRepoPath != "" {
		oldPath, _ = workspaceGitWorkspaceRelPath(gitCtx, oldRepoPath)
	}
	status := "modified"
	switch code {
	case "A":
		status = "added"
	case "D":
		status = "deleted"
	case "R":
		status = "renamed"
	case "C":
		status = "copied"
	case "T":
		status = "typechange"
	}
	return workspaceGitChange{
		Path:        path,
		OldPath:     oldPath,
		Status:      status,
		repoPath:    repoPath,
		oldRepoPath: oldRepoPath,
	}, true
}

func workspaceGitFindChange(changes []workspaceGitChange, path string) (workspaceGitChange, bool) {
	for _, change := range changes {
		if change.Path == path {
			return change, true
		}
	}
	return workspaceGitChange{}, false
}
