package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	workspaceGitScopeWorkingTree    = "working_tree"
	workspaceGitScopeBranch         = "branch"
	workspaceGitScopeCommit         = "commit"
	workspaceGitHistoryRepository   = "repository"
	workspaceGitHistoryFile         = "file"
	workspaceGitCommandTimeout      = 5 * time.Second
	workspaceGitMaxPreviewBytes     = 2 * 1024 * 1024
	workspaceGitHistoryDefaultLimit = 50
	workspaceGitHistoryMaxLimit     = 100
)

type workspaceGitStatusResponse struct {
	Available bool                 `json:"available"`
	Scope     string               `json:"scope"`
	Branch    string               `json:"branch,omitempty"`
	Base      string               `json:"base,omitempty"`
	Target    string               `json:"target,omitempty"`
	Compare   string               `json:"compare,omitempty"`
	Commit    string               `json:"commit,omitempty"`
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
	Commit   string `json:"commit,omitempty"`
	Path     string `json:"path"`
	OldPath  string `json:"old_path,omitempty"`
	Status   string `json:"status"`
	Original string `json:"original"`
	Modified string `json:"modified"`
}

type workspaceGitHistoryResponse struct {
	Available bool                 `json:"available"`
	Branch    string               `json:"branch,omitempty"`
	Mode      string               `json:"mode"`
	Path      string               `json:"path,omitempty"`
	Query     string               `json:"query,omitempty"`
	Limit     int                  `json:"limit"`
	Offset    int                  `json:"offset"`
	HasMore   bool                 `json:"has_more"`
	Message   string               `json:"message,omitempty"`
	Commits   []workspaceGitCommit `json:"commits"`
}

type workspaceGitCommit struct {
	SHA         string `json:"sha"`
	ShortSHA    string `json:"short_sha"`
	Subject     string `json:"subject"`
	AuthorName  string `json:"author_name"`
	AuthorEmail string `json:"author_email"`
	AuthoredAt  string `json:"authored_at"`
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

type workspaceGitHistoryOptions struct {
	mode   string
	path   string
	query  string
	limit  int
	offset int
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
	commit := strings.TrimSpace(r.URL.Query().Get("commit"))
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
	case workspaceGitScopeCommit:
		resolvedCommit, parent, commitMessage, commitErr := workspaceGitCommitSelection(r.Context(), gitCtx, commit)
		if commitErr != nil {
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		response.Commit = resolvedCommit
		response.Compare = resolvedCommit
		response.Base = parent
		response.Target = parent
		if commitMessage != "" {
			response.Available = false
			response.Message = commitMessage
			break
		}
		response.Changes, err = workspaceGitCommitChanges(r.Context(), gitCtx, resolvedCommit, parent)
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
	commit := strings.TrimSpace(r.URL.Query().Get("commit"))
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
		commitRef  string
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
	case workspaceGitScopeCommit:
		var commitMessage string
		commitRef, base, commitMessage, err = workspaceGitCommitSelection(r.Context(), gitCtx, commit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if commitMessage != "" {
			writeError(w, http.StatusBadRequest, commitMessage)
			return
		}
		compareRef = commitRef
		changes, err := workspaceGitCommitChanges(r.Context(), gitCtx, commitRef, base)
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
		Commit:   commitRef,
		Path:     change.Path,
		OldPath:  change.OldPath,
		Status:   change.Status,
		Original: original,
		Modified: modified,
	})
}

func (s *Server) handleWorkspaceGitHistory(w http.ResponseWriter, r *http.Request) {
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

	opts, err := parseWorkspaceGitHistoryOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	gitCtx, message, err := workspaceGitContextForPath(r.Context(), workspace.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if message != "" {
		writeJSON(w, http.StatusOK, workspaceGitHistoryResponse{
			Available: false,
			Mode:      opts.mode,
			Path:      opts.path,
			Query:     opts.query,
			Limit:     opts.limit,
			Offset:    opts.offset,
			Message:   message,
			Commits:   []workspaceGitCommit{},
		})
		return
	}

	commits, hasMore, err := workspaceGitHistory(r.Context(), gitCtx, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, workspaceGitHistoryResponse{
		Available: true,
		Branch:    gitCtx.branch,
		Mode:      opts.mode,
		Path:      opts.path,
		Query:     opts.query,
		Limit:     opts.limit,
		Offset:    opts.offset,
		HasMore:   hasMore,
		Commits:   commits,
	})
}

func cleanWorkspaceGitScope(scope string) string {
	switch strings.TrimSpace(scope) {
	case workspaceGitScopeBranch:
		return workspaceGitScopeBranch
	case workspaceGitScopeCommit:
		return workspaceGitScopeCommit
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
	if scope != workspaceGitScopeBranch && scope != workspaceGitScopeCommit {
		return ""
	}
	return compareRef
}
