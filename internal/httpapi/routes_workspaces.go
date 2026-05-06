package httpapi

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

type workspaceTreeEntry struct {
	Name           string               `json:"name"`
	Path           string               `json:"path"`
	Type           string               `json:"type"`
	HasChildren    bool                 `json:"has_children,omitempty"`
	ChildrenLoaded bool                 `json:"children_loaded,omitempty"`
	Children       []workspaceTreeEntry `json:"children,omitempty"`
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

type workspaceEntryCreateRequest struct {
	Path    string  `json:"path"`
	Type    string  `json:"type"`
	Body    *string `json:"body"`
	Content *string `json:"content"`
}

type workspaceEntryMoveRequest struct {
	Path    string `json:"path"`
	NewPath string `json:"new_path"`
}

type workspaceEntryResponse struct {
	Path string `json:"path"`
	Type string `json:"type"`
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
	relPath := r.URL.Query().Get("path")
	target, err := safeWorkspacePath(workspace.Path, relPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	tree, err := buildWorkspaceTree(root, target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "directory not found")
			return
		}
		if errors.Is(err, errWorkspaceTreePathNotDirectory) {
			writeError(w, http.StatusBadRequest, "path is not a directory")
			return
		}
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
	relPath, err := cleanWorkspaceRelPath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	target, err := safeWorkspacePath(workspace.Path, relPath)
	if err != nil {
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
	writeJSON(w, http.StatusOK, workspaceFileResponse{Path: relPath, Body: body, Content: body})
}

func (s *Server) handleWorkspaceFileContent(w http.ResponseWriter, r *http.Request) {
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
	relPath, err := cleanWorkspaceRelPath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	target, err := safeWorkspacePath(workspace.Path, relPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	file, err := os.Open(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if stat.IsDir() {
		writeError(w, http.StatusBadRequest, "path is a directory")
		return
	}

	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Type", workspaceFileContentType(relPath, file))
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filepath.Base(relPath)}))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filepath.Base(relPath), stat.ModTime(), file)
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
	relPath, err := cleanWorkspaceRelPath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	target, err := safeWorkspacePath(workspace.Path, relPath)
	if err != nil {
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
	writeJSON(w, http.StatusOK, workspaceFileResponse{Path: relPath, Body: content, Content: content})
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
	relPath, err := cleanWorkspaceRelPath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	target, err := safeWorkspacePath(workspace.Path, relPath)
	if err != nil {
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

func (s *Server) handleCreateWorkspaceEntry(w http.ResponseWriter, r *http.Request) {
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
	var req workspaceEntryCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	relPath, err := cleanWorkspaceRelPath(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	target, err := safeWorkspacePath(workspace.Path, relPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	entryType := strings.ToLower(strings.TrimSpace(req.Type))
	if entryType == "" {
		entryType = "file"
	}
	if entryType != "file" && entryType != "directory" {
		writeError(w, http.StatusBadRequest, "entry type must be file or directory")
		return
	}
	if _, err := os.Lstat(target); err == nil {
		writeError(w, http.StatusConflict, "entry already exists")
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if entryType == "directory" {
		if err := os.MkdirAll(target, 0o755); err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		writeJSON(w, http.StatusCreated, workspaceEntryResponse{Path: relPath, Type: entryType})
		return
	}
	content := ""
	switch {
	case req.Body != nil:
		content = *req.Body
	case req.Content != nil:
		content = *req.Content
	}
	if strings.ContainsRune(content, 0) || !utf8.ValidString(content) {
		writeError(w, http.StatusUnsupportedMediaType, "binary files are not supported")
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			writeError(w, http.StatusConflict, "entry already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		_ = os.Remove(target)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if err := file.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, workspaceEntryResponse{Path: relPath, Type: entryType})
}

func (s *Server) handleMoveWorkspaceEntry(w http.ResponseWriter, r *http.Request) {
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
	var req workspaceEntryMoveRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	relPath, err := cleanWorkspaceRelPath(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	newRelPath, err := cleanWorkspaceRelPath(req.NewPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid target path")
		return
	}
	source, err := safeWorkspacePath(workspace.Path, relPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	target, err := safeWorkspacePath(workspace.Path, newRelPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid target path")
		return
	}
	info, err := os.Lstat(source)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "entry not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if relPath == newRelPath {
		writeJSON(w, http.StatusOK, workspaceEntryResponse{Path: newRelPath, Type: workspaceEntryType(info)})
		return
	}
	if info.IsDir() && strings.HasPrefix(newRelPath, relPath+"/") {
		writeError(w, http.StatusBadRequest, "directory cannot be moved into itself")
		return
	}
	if _, err := os.Lstat(target); err == nil {
		writeError(w, http.StatusConflict, "entry already exists")
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	parentInfo, err := os.Stat(filepath.Dir(target))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "target directory not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !parentInfo.IsDir() {
		writeError(w, http.StatusBadRequest, "target parent is not a directory")
		return
	}
	if err := os.Rename(source, target); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, workspaceEntryResponse{Path: newRelPath, Type: workspaceEntryType(info)})
}

func (s *Server) handleDeleteWorkspaceEntry(w http.ResponseWriter, r *http.Request) {
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
	relPath, err := cleanWorkspaceRelPath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	target, err := safeWorkspacePath(workspace.Path, relPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	info, err := os.Lstat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "entry not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if info.IsDir() {
		if err := os.RemoveAll(target); err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
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

func cleanWorkspaceRelPath(relPath string) (string, error) {
	trimmed := strings.TrimSpace(relPath)
	if trimmed == "" {
		return "", errors.New("empty path")
	}
	if filepath.IsAbs(trimmed) {
		return "", errors.New("absolute paths are not allowed")
	}
	cleanRel := filepath.Clean(trimmed)
	if cleanRel == "." {
		return "", errors.New("empty path")
	}
	for _, part := range strings.Split(cleanRel, string(filepath.Separator)) {
		if part == ".." {
			return "", errors.New("parent traversal is not allowed")
		}
	}
	return filepath.ToSlash(cleanRel), nil
}

func workspaceEntryType(info os.FileInfo) string {
	if info.IsDir() {
		return "directory"
	}
	return "file"
}

func workspaceFileContentType(relPath string, file *os.File) string {
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(relPath))); contentType != "" {
		return contentType
	}
	var sample [512]byte
	n, err := file.Read(sample[:])
	if err == nil || errors.Is(err, io.EOF) {
		if _, seekErr := file.Seek(0, io.SeekStart); seekErr == nil && n > 0 {
			return http.DetectContentType(sample[:n])
		}
	}
	_, _ = file.Seek(0, io.SeekStart)
	return "application/octet-stream"
}

var errWorkspaceTreePathNotDirectory = errors.New("workspace tree path is not a directory")

func buildWorkspaceTree(root string, current string) (workspaceTreeEntry, error) {
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
		return workspaceTreeEntry{}, errWorkspaceTreePathNotDirectory
	}
	entry.Type = "directory"
	entry.ChildrenLoaded = true
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
		childEntry, err := buildWorkspaceTreeChild(root, filepath.Join(current, child.Name()))
		if err != nil {
			continue
		}
		entry.Children = append(entry.Children, childEntry)
	}
	entry.HasChildren = len(entry.Children) > 0
	return entry, nil
}

func buildWorkspaceTreeChild(root string, current string) (workspaceTreeEntry, error) {
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
	if !info.IsDir() {
		return entry, nil
	}
	entry.Type = "directory"
	entry.HasChildren = workspaceDirectoryHasVisibleChildren(current)
	return entry, nil
}

func workspaceDirectoryHasVisibleChildren(path string) bool {
	children, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, child := range children {
		if !strings.HasPrefix(child.Name(), ".") {
			return true
		}
	}
	return false
}
