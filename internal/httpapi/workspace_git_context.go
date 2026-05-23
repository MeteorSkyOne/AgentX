package httpapi

import (
	"context"
	"path/filepath"
	"strings"
)

func workspaceGitContextForPath(ctx context.Context, workspacePath string) (workspaceGitContext, string, error) {
	workspaceRoot, err := safeWorkspacePath(workspacePath, "")
	if err != nil {
		return workspaceGitContext{}, "", err
	}
	workspaceRoot, err = filepath.Abs(workspaceRoot)
	if err != nil {
		return workspaceGitContext{}, "", err
	}
	repoRoot, err := workspaceGitOutput(ctx, workspaceRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return workspaceGitContext{}, "workspace is not inside a git repository", nil
	}
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return workspaceGitContext{}, "workspace is not inside a git repository", nil
	}
	repoRoot, err = filepath.Abs(repoRoot)
	if err != nil {
		return workspaceGitContext{}, "", err
	}
	prefix, err := filepath.Rel(repoRoot, workspaceRoot)
	if err != nil {
		return workspaceGitContext{}, "", err
	}
	if prefix == "." {
		prefix = ""
	}
	if prefix == ".." || strings.HasPrefix(prefix, ".."+string(filepath.Separator)) {
		return workspaceGitContext{}, "workspace is not inside a git repository", nil
	}
	branch, _ := workspaceGitOutput(ctx, repoRoot, "branch", "--show-current")
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "HEAD"
	}
	return workspaceGitContext{
		workspaceRoot: workspaceRoot,
		repoRoot:      repoRoot,
		prefix:        filepath.ToSlash(prefix),
		branch:        branch,
	}, "", nil
}

func workspaceGitBranchSelection(ctx context.Context, gitCtx workspaceGitContext, requestedBase string, requestedCompare string) (string, string, []workspaceGitTarget, string, error) {
	targets, err := workspaceGitBranchTargets(ctx, gitCtx)
	if err != nil {
		return "", "", nil, "", err
	}
	if len(targets) == 0 {
		return "", "", targets, "branch diff is unavailable because no branch was found", nil
	}
	targetNames := make(map[string]bool, len(targets))
	for _, target := range targets {
		targetNames[target.Name] = true
	}

	requestedCompare = strings.TrimSpace(requestedCompare)
	compare := requestedCompare
	if compare == "" {
		compare = gitCtx.branch
	}
	if compare == "" {
		compare = "HEAD"
	}
	if compare != "HEAD" && !targetNames[compare] {
		return "", "", targets, "compare branch was not found", nil
	}
	if err := workspaceGitVerifyCommitRef(ctx, gitCtx.repoRoot, compare); err != nil {
		return "", "", targets, "compare branch was not found", nil
	}

	candidates := make([]string, 0, len(targets)+5)
	if requestedBase = strings.TrimSpace(requestedBase); requestedBase != "" {
		if requestedBase != "HEAD" && !targetNames[requestedBase] {
			return "", "", targets, "base branch was not found", nil
		}
		candidates = append(candidates, requestedBase)
	} else {
		if upstream, err := workspaceGitOutput(ctx, gitCtx.repoRoot, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
			if upstream = strings.TrimSpace(upstream); upstream != "" {
				candidates = append(candidates, upstream)
			}
		}
		candidates = append(candidates, "origin/main", "origin/master", "main", "master")
		for _, target := range targets {
			candidates = append(candidates, target.Name)
		}
	}
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		if requestedBase == "" && candidate == compare {
			continue
		}
		if candidate != "HEAD" && !targetNames[candidate] {
			continue
		}
		if err := workspaceGitVerifyCommitRef(ctx, gitCtx.repoRoot, candidate); err != nil {
			continue
		}
		for i := range targets {
			targets[i].Default = targets[i].Name == candidate
		}
		return candidate, compare, targets, "", nil
	}
	if requestedBase != "" {
		return "", "", targets, "base branch was not found", nil
	}
	return "", "", targets, "branch diff is unavailable because no base branch was found", nil
}

func workspaceGitBranchTargets(ctx context.Context, gitCtx workspaceGitContext) ([]workspaceGitTarget, error) {
	output, err := workspaceGitOutput(ctx, gitCtx.repoRoot, "for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var targets []workspaceGitTarget
	for _, line := range strings.Split(output, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || name == "HEAD" || strings.HasSuffix(name, "/HEAD") || seen[name] {
			continue
		}
		seen[name] = true
		targets = append(targets, workspaceGitTarget{Name: name})
	}
	return targets, nil
}

func workspaceGitVerifyCommitRef(ctx context.Context, repoRoot string, ref string) error {
	return workspaceGitRun(ctx, repoRoot, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
}

func workspaceGitResolveCommit(ctx context.Context, repoRoot string, ref string) (string, error) {
	output, err := workspaceGitOutput(ctx, repoRoot, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func workspaceGitCommitSelection(ctx context.Context, gitCtx workspaceGitContext, requestedCommit string) (string, string, string, error) {
	requestedCommit = strings.TrimSpace(requestedCommit)
	if requestedCommit == "" {
		return "", "", "commit is required", nil
	}
	commit, err := workspaceGitResolveCommit(ctx, gitCtx.repoRoot, requestedCommit)
	if err != nil || commit == "" {
		return "", "", "commit was not found", nil
	}
	parent, err := workspaceGitFirstParent(ctx, gitCtx.repoRoot, commit)
	if err != nil {
		return "", "", "", err
	}
	return commit, parent, "", nil
}

func workspaceGitFirstParent(ctx context.Context, repoRoot string, commit string) (string, error) {
	output, err := workspaceGitOutput(ctx, repoRoot, "rev-list", "--parents", "-n", "1", commit)
	if err != nil {
		return "", err
	}
	parts := strings.Fields(output)
	if len(parts) < 2 {
		return "", nil
	}
	return parts[1], nil
}
