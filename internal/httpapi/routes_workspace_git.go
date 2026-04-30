package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

const (
	workspaceGitScopeWorkingTree = "working_tree"
	workspaceGitScopeBranch      = "branch"
	workspaceGitCommandTimeout   = 5 * time.Second
	workspaceGitMaxPreviewBytes  = 2 * 1024 * 1024
)

type workspaceGitStatusResponse struct {
	Available bool                 `json:"available"`
	Scope     string               `json:"scope"`
	Branch    string               `json:"branch,omitempty"`
	Base      string               `json:"base,omitempty"`
	Target    string               `json:"target,omitempty"`
	Compare   string               `json:"compare,omitempty"`
	Targets   []workspaceGitTarget `json:"targets,omitempty"`
	Message   string               `json:"message,omitempty"`
	Changes   []workspaceGitChange `json:"changes"`
}

type workspaceGitDiffResponse struct {
	Scope    string `json:"scope"`
	Branch   string `json:"branch,omitempty"`
	Base     string `json:"base,omitempty"`
	Target   string `json:"target,omitempty"`
	Compare  string `json:"compare,omitempty"`
	Path     string `json:"path"`
	OldPath  string `json:"old_path,omitempty"`
	Status   string `json:"status"`
	Original string `json:"original"`
	Modified string `json:"modified"`
}

type workspaceGitTarget struct {
	Name    string `json:"name"`
	Default bool   `json:"default,omitempty"`
}

type workspaceGitChange struct {
	Path        string `json:"path"`
	OldPath     string `json:"old_path,omitempty"`
	Status      string `json:"status"`
	Staged      bool   `json:"staged,omitempty"`
	Unstaged    bool   `json:"unstaged,omitempty"`
	Untracked   bool   `json:"untracked,omitempty"`
	IndexStatus string `json:"index_status,omitempty"`
	WorkStatus  string `json:"work_status,omitempty"`

	repoPath    string
	oldRepoPath string
}

type workspaceGitContext struct {
	workspaceRoot string
	repoRoot      string
	prefix        string
	branch        string
}

func (s *Server) handleWorkspaceGitStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	workspace, ok, err := s.authorizedWorkspace(r, userID, chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	scope := cleanWorkspaceGitScope(r.URL.Query().Get("scope"))
	target := cleanWorkspaceGitBaseParam(r)
	compare := strings.TrimSpace(r.URL.Query().Get("compare"))
	gitCtx, message, err := workspaceGitContextForPath(r.Context(), workspace.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if message != "" {
		writeJSON(w, http.StatusOK, workspaceGitStatusResponse{
			Available: false,
			Scope:     scope,
			Message:   message,
			Changes:   []workspaceGitChange{},
		})
		return
	}

	response := workspaceGitStatusResponse{
		Available: true,
		Scope:     scope,
		Branch:    gitCtx.branch,
		Changes:   []workspaceGitChange{},
	}
	switch scope {
	case workspaceGitScopeWorkingTree:
		response.Changes, err = workspaceGitWorkingTreeChanges(r.Context(), gitCtx)
	case workspaceGitScopeBranch:
		base, compareRef, targets, baseMessage, baseErr := workspaceGitBranchSelection(r.Context(), gitCtx, target, compare)
		if baseErr != nil {
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		response.Branch = compareRef
		response.Base = base
		response.Target = base
		response.Compare = compareRef
		response.Targets = targets
		if baseMessage != "" {
			response.Available = false
			response.Message = baseMessage
			break
		}
		response.Changes, err = workspaceGitBranchChanges(r.Context(), gitCtx, base, compareRef)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleWorkspaceGitDiff(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	workspace, ok, err := s.authorizedWorkspace(r, userID, chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	scope := cleanWorkspaceGitScope(r.URL.Query().Get("scope"))
	target := cleanWorkspaceGitBaseParam(r)
	compare := strings.TrimSpace(r.URL.Query().Get("compare"))
	relPath, err := cleanWorkspaceRelPath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	gitCtx, message, err := workspaceGitContextForPath(r.Context(), workspace.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if message != "" {
		writeError(w, http.StatusBadRequest, message)
		return
	}

	var (
		change     workspaceGitChange
		base       string
		compareRef = "HEAD"
		found      bool
	)
	switch scope {
	case workspaceGitScopeWorkingTree:
		changes, err := workspaceGitWorkingTreeChanges(r.Context(), gitCtx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		change, found = workspaceGitFindChange(changes, relPath)
	case workspaceGitScopeBranch:
		var baseMessage string
		base, compareRef, _, baseMessage, err = workspaceGitBranchSelection(r.Context(), gitCtx, target, compare)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if baseMessage != "" {
			writeError(w, http.StatusBadRequest, baseMessage)
			return
		}
		changes, err := workspaceGitBranchChanges(r.Context(), gitCtx, base, compareRef)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		change, found = workspaceGitFindChange(changes, relPath)
	}
	if !found {
		writeError(w, http.StatusNotFound, "change not found")
		return
	}

	original, modified, err := workspaceGitDiffContents(r.Context(), gitCtx, scope, base, compareRef, change)
	if err != nil {
		switch {
		case errors.Is(err, errWorkspaceGitBinary):
			writeError(w, http.StatusUnsupportedMediaType, "binary files are not supported")
		case errors.Is(err, errWorkspaceGitTooLarge):
			writeError(w, http.StatusRequestEntityTooLarge, "file is too large to preview")
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, workspaceGitDiffResponse{
		Scope:    scope,
		Branch:   workspaceGitResponseBranch(scope, gitCtx, compareRef),
		Base:     base,
		Target:   base,
		Compare:  workspaceGitResponseCompare(scope, compareRef),
		Path:     change.Path,
		OldPath:  change.OldPath,
		Status:   change.Status,
		Original: original,
		Modified: modified,
	})
}

func cleanWorkspaceGitScope(scope string) string {
	switch strings.TrimSpace(scope) {
	case workspaceGitScopeBranch:
		return workspaceGitScopeBranch
	default:
		return workspaceGitScopeWorkingTree
	}
}

func cleanWorkspaceGitBaseParam(r *http.Request) string {
	if base := strings.TrimSpace(r.URL.Query().Get("base")); base != "" {
		return base
	}
	return strings.TrimSpace(r.URL.Query().Get("target"))
}

func workspaceGitResponseBranch(scope string, gitCtx workspaceGitContext, compareRef string) string {
	if scope == workspaceGitScopeBranch && compareRef != "" {
		return compareRef
	}
	return gitCtx.branch
}

func workspaceGitResponseCompare(scope string, compareRef string) string {
	if scope != workspaceGitScopeBranch {
		return ""
	}
	return compareRef
}

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

func workspaceGitDiffContents(ctx context.Context, gitCtx workspaceGitContext, scope string, baseRef string, compareRef string, change workspaceGitChange) (string, string, error) {
	switch scope {
	case workspaceGitScopeBranch:
		original := ""
		modified := ""
		var err error
		original, _, err = workspaceGitTextAtRefOptional(ctx, gitCtx.repoRoot, baseRef, workspaceGitOriginalRepoPath(change))
		if err != nil {
			return "", "", err
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

var (
	errWorkspaceGitBinary   = errors.New("workspace git preview is binary")
	errWorkspaceGitTooLarge = errors.New("workspace git preview is too large")
)

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
