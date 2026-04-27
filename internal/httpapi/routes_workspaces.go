package httpapi

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

type workspaceTreeEntry struct {
	Name     string               `json:"name"`
	Path     string               `json:"path"`
	Type     string               `json:"type"`
	Children []workspaceTreeEntry `json:"children,omitempty"`
}

type workspaceFileResponse struct {
	Path    string `json:"path"`
	Body    string `json:"body"`
	Content string `json:"content"`
}

type workspaceFileRequest struct {
	Body    *string `json:"body"`
	Content *string `json:"content"`
}

func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, workspace)
}

func (s *Server) handleWorkspaceTree(w http.ResponseWriter, r *http.Request) {
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
	root, err := safeWorkspacePath(workspace.Path, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	tree, err := buildWorkspaceTree(root, root, 4)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, tree)
}

func (s *Server) handleWorkspaceFile(w http.ResponseWriter, r *http.Request) {
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
	relPath := r.URL.Query().Get("path")
	target, err := safeWorkspacePath(workspace.Path, relPath)
	if err != nil || strings.TrimSpace(relPath) == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	data, err := os.ReadFile(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if bytes.Contains(data, []byte{0}) || !utf8.Valid(data) {
		writeError(w, http.StatusUnsupportedMediaType, "binary files are not supported")
		return
	}
	body := string(data)
	writeJSON(w, http.StatusOK, workspaceFileResponse{Path: filepath.ToSlash(filepath.Clean(relPath)), Body: body, Content: body})
}

func (s *Server) handlePutWorkspaceFile(w http.ResponseWriter, r *http.Request) {
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
	relPath := r.URL.Query().Get("path")
	target, err := safeWorkspacePath(workspace.Path, relPath)
	if err != nil || strings.TrimSpace(relPath) == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	var req workspaceFileRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	content := ""
	switch {
	case req.Body != nil:
		content = *req.Body
	case req.Content != nil:
		content = *req.Content
	default:
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}
	if strings.ContainsRune(content, 0) || !utf8.ValidString(content) {
		writeError(w, http.StatusUnsupportedMediaType, "binary files are not supported")
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, workspaceFileResponse{Path: filepath.ToSlash(filepath.Clean(relPath)), Body: content, Content: content})
}

func (s *Server) handleDeleteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
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
	relPath := r.URL.Query().Get("path")
	target, err := safeWorkspacePath(workspace.Path, relPath)
	if err != nil || strings.TrimSpace(relPath) == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "directories cannot be deleted here")
		return
	}
	if err := os.Remove(target); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func safeWorkspacePath(root string, relPath string) (string, error) {
	if root == "" {
		return "", errors.New("empty workspace root")
	}
	if filepath.IsAbs(relPath) {
		return "", errors.New("absolute paths are not allowed")
	}
	cleanRel := filepath.Clean(strings.TrimSpace(relPath))
	if cleanRel == "." {
		cleanRel = ""
	}
	for _, part := range strings.Split(cleanRel, string(filepath.Separator)) {
		if part == ".." {
			return "", errors.New("parent traversal is not allowed")
		}
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, cleanRel))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes workspace")
	}
	return targetAbs, nil
}

func buildWorkspaceTree(root string, current string, maxDepth int) (workspaceTreeEntry, error) {
	rel, err := filepath.Rel(root, current)
	if err != nil {
		return workspaceTreeEntry{}, err
	}
	if rel == "." {
		rel = ""
	}
	info, err := os.Stat(current)
	if err != nil {
		return workspaceTreeEntry{}, err
	}
	entry := workspaceTreeEntry{
		Name: info.Name(),
		Path: filepath.ToSlash(rel),
		Type: "file",
	}
	if rel == "" {
		entry.Name = ""
	}
	if !info.IsDir() {
		return entry, nil
	}
	entry.Type = "directory"
	if maxDepth <= 0 {
		return entry, nil
	}
	children, err := os.ReadDir(current)
	if err != nil {
		return entry, nil
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].IsDir() != children[j].IsDir() {
			return children[i].IsDir()
		}
		return children[i].Name() < children[j].Name()
	})
	for _, child := range children {
		if strings.HasPrefix(child.Name(), ".") {
			continue
		}
		childEntry, err := buildWorkspaceTree(root, filepath.Join(current, child.Name()), maxDepth-1)
		if err != nil {
			continue
		}
		entry.Children = append(entry.Children, childEntry)
	}
	return entry, nil
}
