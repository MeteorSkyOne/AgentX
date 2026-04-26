package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
)

type namedRequest struct {
	Name string `json:"name"`
}

type channelRequest struct {
	Name string             `json:"name"`
	Type domain.ChannelType `json:"type"`
}

type threadCreateRequest struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	InitialBody string `json:"initial_body"`
}

type threadUpdateRequest struct {
	Title string `json:"title"`
}

type projectUpdateRequest struct {
	Name          *string `json:"name"`
	WorkspacePath *string `json:"workspace_path"`
}

type notificationSettingsRequest struct {
	WebhookEnabled bool    `json:"webhook_enabled"`
	WebhookURL     string  `json:"webhook_url"`
	WebhookSecret  *string `json:"webhook_secret"`
}

type notificationTestResponse struct {
	OK bool `json:"ok"`
}

type agentCreateRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Handle      string            `json:"handle"`
	Kind        string            `json:"kind"`
	Model       string            `json:"model"`
	Effort      string            `json:"effort"`
	FastMode    bool              `json:"fast_mode"`
	YoloMode    bool              `json:"yolo_mode"`
	Env         map[string]string `json:"env"`
}

type agentUpdateRequest struct {
	Name        *string           `json:"name"`
	Description *string           `json:"description"`
	Handle      *string           `json:"handle"`
	Kind        *string           `json:"kind"`
	Model       *string           `json:"model"`
	Effort      *string           `json:"effort"`
	Enabled     *bool             `json:"enabled"`
	FastMode    *bool             `json:"fast_mode"`
	YoloMode    *bool             `json:"yolo_mode"`
	Env         map[string]string `json:"env"`
}

type channelAgentsRequest struct {
	Agents []channelAgentRequest `json:"agents"`
}

type channelAgentRequest struct {
	AgentID        string `json:"agent_id"`
	RunWorkspaceID string `json:"run_workspace_id"`
}

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

func (s *Server) handleNotificationSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if authorized, err := s.authorizedOrganization(r, userID, orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	} else if !authorized {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	settings, err := s.app.NotificationSettings(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleUpdateNotificationSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if authorized, err := s.authorizedOrganization(r, userID, orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	} else if !authorized {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	var req notificationSettingsRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	settings, err := s.app.UpdateNotificationSettings(r.Context(), orgID, app.NotificationSettingsUpdateRequest{
		WebhookEnabled: req.WebhookEnabled,
		WebhookURL:     req.WebhookURL,
		WebhookSecret:  req.WebhookSecret,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleTestNotificationSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if authorized, err := s.authorizedOrganization(r, userID, orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	} else if !authorized {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	if err := s.app.TestNotificationSettings(r.Context(), orgID); err != nil {
		if errors.Is(err, app.ErrWebhookDeliveryFailed) {
			writeError(w, http.StatusBadGateway, "webhook delivery failed")
			return
		}
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, notificationTestResponse{OK: true})
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if authorized, err := s.authorizedOrganization(r, userID, orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	} else if !authorized {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	projects, err := s.app.ListProjects(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if authorized, err := s.authorizedOrganization(r, userID, orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	} else if !authorized {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	var req namedRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	project, err := s.app.CreateProject(r.Context(), userID, orgID, req.Name)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	project, ok, err := s.authorizedProject(r, userID, chi.URLParam(r, "projectID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var req projectUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	updated, err := s.app.UpdateProject(r.Context(), project.ID, app.ProjectUpdateRequest{
		Name:          req.Name,
		WorkspacePath: req.WorkspacePath,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	project, ok, err := s.authorizedProject(r, userID, chi.URLParam(r, "projectID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err := s.app.DeleteProject(r.Context(), project.ID); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleProjectChannels(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	project, ok, err := s.authorizedProject(r, userID, chi.URLParam(r, "projectID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	channels, err := s.app.ListProjectChannels(r.Context(), project.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, channels)
}

func (s *Server) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	project, ok, err := s.authorizedProject(r, userID, chi.URLParam(r, "projectID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var req channelRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	channel, err := s.app.CreateChannel(r.Context(), project.ID, req.Name, req.Type)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, channel)
}

func (s *Server) handleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	channel, ok, err := s.authorizedChannel(r, userID, chi.URLParam(r, "channelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	var req channelRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	updated, err := s.app.UpdateChannel(r.Context(), channel.ID, req.Name, req.Type)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleArchiveChannel(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	channel, ok, err := s.authorizedChannel(r, userID, chi.URLParam(r, "channelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	if err := s.app.ArchiveChannel(r.Context(), channel.ID); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleThreads(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	channel, ok, err := s.authorizedChannel(r, userID, chi.URLParam(r, "channelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	threads, err := s.app.ListThreads(r.Context(), channel.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, threads)
}

func (s *Server) handleCreateThread(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	channel, ok, err := s.authorizedChannel(r, userID, chi.URLParam(r, "channelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	var req threadCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	body := req.Body
	if body == "" {
		body = req.InitialBody
	}
	thread, message, err := s.app.CreateThread(r.Context(), userID, channel.ID, req.Title, body)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thread": thread, "message": message})
}

func (s *Server) handleUpdateThread(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	thread, ok, err := s.authorizedThread(r, userID, chi.URLParam(r, "threadID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	var req threadUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	updated, err := s.app.UpdateThread(r.Context(), thread.ID, req.Title)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleArchiveThread(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	thread, ok, err := s.authorizedThread(r, userID, chi.URLParam(r, "threadID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	if err := s.app.ArchiveThread(r.Context(), thread.ID); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if authorized, err := s.authorizedOrganization(r, userID, orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	} else if !authorized {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	agents, err := s.app.ListAgents(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	for i := range agents {
		agents[i] = redactAgent(agents[i])
	}
	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID := chi.URLParam(r, "orgID")
	if authorized, err := s.authorizedOrganization(r, userID, orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	} else if !authorized {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	var req agentCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	agent, err := s.app.CreateAgent(r.Context(), app.AgentCreateRequest{
		UserID:         userID,
		OrganizationID: orgID,
		Name:           req.Name,
		Description:    req.Description,
		Handle:         req.Handle,
		Kind:           req.Kind,
		Model:          req.Model,
		Effort:         req.Effort,
		FastMode:       req.FastMode,
		YoloMode:       req.YoloMode,
		Env:            req.Env,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, redactAgent(agent))
}

func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	agent, ok, err := s.authorizedAgent(r, userID, chi.URLParam(r, "agentID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	var req agentUpdateRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	updated, err := s.app.UpdateAgent(r.Context(), agent.ID, app.AgentUpdateRequest{
		Name:        req.Name,
		Description: req.Description,
		Handle:      req.Handle,
		Kind:        req.Kind,
		Model:       req.Model,
		Effort:      req.Effort,
		Enabled:     req.Enabled,
		FastMode:    req.FastMode,
		YoloMode:    req.YoloMode,
		Env:         req.Env,
		EnvSet:      req.Env != nil,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, redactAgent(updated))
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	agent, ok, err := s.authorizedAgent(r, userID, chi.URLParam(r, "agentID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if err := s.app.DeleteAgent(r.Context(), agent.ID); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleChannelAgents(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	channel, ok, err := s.authorizedChannel(r, userID, chi.URLParam(r, "channelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	agents, err := s.app.ChannelAgents(r.Context(), channel.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, redactConversationAgents(agents))
}

func (s *Server) handleSetChannelAgents(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	channel, ok, err := s.authorizedChannel(r, userID, chi.URLParam(r, "channelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	var req channelAgentsRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	bindings := make([]domain.ChannelAgent, 0, len(req.Agents))
	for _, item := range req.Agents {
		if strings.TrimSpace(item.AgentID) == "" {
			writeError(w, http.StatusBadRequest, "agent_id is required")
			return
		}
		bindings = append(bindings, domain.ChannelAgent{
			AgentID:        strings.TrimSpace(item.AgentID),
			RunWorkspaceID: strings.TrimSpace(item.RunWorkspaceID),
		})
	}
	agents, err := s.app.SetChannelAgents(r.Context(), channel.ID, bindings)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, redactConversationAgents(agents))
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

func (s *Server) authorizedProject(r *http.Request, userID string, projectID string) (domain.Project, bool, error) {
	project, err := s.app.Project(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Project{}, false, nil
		}
		return domain.Project{}, false, err
	}
	if authorized, err := s.authorizedOrganization(r, userID, project.OrganizationID); err != nil || !authorized {
		return domain.Project{}, authorized, err
	}
	return project, true, nil
}

func (s *Server) authorizedChannel(r *http.Request, userID string, channelID string) (domain.Channel, bool, error) {
	channel, err := s.app.Channel(r.Context(), channelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Channel{}, false, nil
		}
		return domain.Channel{}, false, err
	}
	if authorized, err := s.authorizedOrganization(r, userID, channel.OrganizationID); err != nil || !authorized {
		return domain.Channel{}, authorized, err
	}
	return channel, true, nil
}

func (s *Server) authorizedThread(r *http.Request, userID string, threadID string) (domain.Thread, bool, error) {
	thread, err := s.app.Thread(r.Context(), threadID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Thread{}, false, nil
		}
		return domain.Thread{}, false, err
	}
	if authorized, err := s.authorizedOrganization(r, userID, thread.OrganizationID); err != nil || !authorized {
		return domain.Thread{}, authorized, err
	}
	return thread, true, nil
}

func (s *Server) authorizedAgent(r *http.Request, userID string, agentID string) (domain.Agent, bool, error) {
	agent, err := s.app.Agent(r.Context(), agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Agent{}, false, nil
		}
		return domain.Agent{}, false, err
	}
	if authorized, err := s.authorizedOrganization(r, userID, agent.OrganizationID); err != nil || !authorized {
		return domain.Agent{}, authorized, err
	}
	return agent, true, nil
}

func (s *Server) authorizedWorkspace(r *http.Request, userID string, workspaceID string) (domain.Workspace, bool, error) {
	workspace, err := s.app.Workspace(r.Context(), workspaceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Workspace{}, false, nil
		}
		return domain.Workspace{}, false, err
	}
	if authorized, err := s.authorizedOrganization(r, userID, workspace.OrganizationID); err != nil || !authorized {
		return domain.Workspace{}, authorized, err
	}
	return workspace, true, nil
}

func writeAppError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid input")
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, "not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func redactConversationContext(ctx app.ConversationContext) app.ConversationContext {
	for i := range ctx.Agents {
		ctx.Agents[i].Agent = redactAgent(ctx.Agents[i].Agent)
	}
	ctx.Agent = redactAgent(ctx.Agent)
	return ctx
}

func redactConversationAgents(agents []app.ConversationAgentContext) []app.ConversationAgentContext {
	for i := range agents {
		agents[i].Agent = redactAgent(agents[i].Agent)
	}
	return agents
}

func redactAgent(agent domain.Agent) domain.Agent {
	if len(agent.Env) == 0 {
		agent.Env = map[string]string{}
		return agent
	}
	redacted := make(map[string]string, len(agent.Env))
	for key, value := range agent.Env {
		if value == "" {
			redacted[key] = ""
			continue
		}
		redacted[key] = "********"
	}
	agent.Env = redacted
	return agent
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
