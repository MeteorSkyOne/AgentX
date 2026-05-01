package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
)

type scheduledTaskRequest struct {
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	Enabled          bool   `json:"enabled"`
	Schedule         string `json:"schedule"`
	Timezone         string `json:"timezone"`
	ConversationType string `json:"conversation_type"`
	ConversationID   string `json:"conversation_id"`
	AgentID          string `json:"agent_id"`
	WorkspaceID      string `json:"workspace_id"`
	Prompt           string `json:"prompt"`
	Command          string `json:"command"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
}

type scheduledTaskUpdateRequest struct {
	Name             *string `json:"name"`
	Kind             *string `json:"kind"`
	Enabled          *bool   `json:"enabled"`
	Schedule         *string `json:"schedule"`
	Timezone         *string `json:"timezone"`
	ConversationType *string `json:"conversation_type"`
	ConversationID   *string `json:"conversation_id"`
	AgentID          *string `json:"agent_id"`
	WorkspaceID      *string `json:"workspace_id"`
	Prompt           *string `json:"prompt"`
	Command          *string `json:"command"`
	TimeoutSeconds   *int    `json:"timeout_seconds"`
}

func (s *Server) handleProjectScheduledTasks(w http.ResponseWriter, r *http.Request) {
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
	tasks, err := s.app.ListScheduledTasks(r.Context(), project.ID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) handleCreateScheduledTask(w http.ResponseWriter, r *http.Request) {
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
	var req scheduledTaskRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	kind, ok := parseScheduledTaskKind(req.Kind)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown task kind")
		return
	}
	if kind == domain.ScheduledTaskKindShellCommand && !s.authorizedShellTaskManagement(w, r, userID, project.OrganizationID) {
		return
	}
	conversationType, ok := parseOptionalConversationType(req.ConversationType)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown conversation type")
		return
	}
	task, err := s.app.CreateScheduledTask(r.Context(), app.ScheduledTaskCreateRequest{
		UserID:           userID,
		ProjectID:        project.ID,
		Name:             req.Name,
		Kind:             kind,
		Enabled:          req.Enabled,
		Schedule:         req.Schedule,
		Timezone:         req.Timezone,
		ConversationType: conversationType,
		ConversationID:   req.ConversationID,
		AgentID:          req.AgentID,
		WorkspaceID:      req.WorkspaceID,
		Prompt:           req.Prompt,
		Command:          req.Command,
		TimeoutSeconds:   req.TimeoutSeconds,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleUpdateScheduledTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	task, ok, err := s.authorizedScheduledTask(r, userID, chi.URLParam(r, "taskID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "scheduled task not found")
		return
	}
	var req scheduledTaskUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	var kind *domain.ScheduledTaskKind
	if req.Kind != nil {
		parsed, ok := parseScheduledTaskKind(*req.Kind)
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown task kind")
			return
		}
		kind = &parsed
	}
	if (task.Kind == domain.ScheduledTaskKindShellCommand || (kind != nil && *kind == domain.ScheduledTaskKindShellCommand)) &&
		!s.authorizedShellTaskManagement(w, r, userID, task.OrganizationID) {
		return
	}
	var conversationType *domain.ConversationType
	if req.ConversationType != nil {
		parsed, ok := parseOptionalConversationType(*req.ConversationType)
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown conversation type")
			return
		}
		conversationType = &parsed
	}
	updated, err := s.app.UpdateScheduledTask(r.Context(), task.ID, app.ScheduledTaskUpdateRequest{
		Name:             req.Name,
		Kind:             kind,
		Enabled:          req.Enabled,
		Schedule:         req.Schedule,
		Timezone:         req.Timezone,
		ConversationType: conversationType,
		ConversationID:   req.ConversationID,
		AgentID:          req.AgentID,
		WorkspaceID:      req.WorkspaceID,
		Prompt:           req.Prompt,
		Command:          req.Command,
		TimeoutSeconds:   req.TimeoutSeconds,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteScheduledTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	task, ok, err := s.authorizedScheduledTask(r, userID, chi.URLParam(r, "taskID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "scheduled task not found")
		return
	}
	if task.Kind == domain.ScheduledTaskKindShellCommand && !s.authorizedShellTaskManagement(w, r, userID, task.OrganizationID) {
		return
	}
	if err := s.app.DeleteScheduledTask(r.Context(), task.ID); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRunScheduledTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	task, ok, err := s.authorizedScheduledTask(r, userID, chi.URLParam(r, "taskID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "scheduled task not found")
		return
	}
	if task.Kind == domain.ScheduledTaskKindShellCommand && !s.authorizedShellTaskManagement(w, r, userID, task.OrganizationID) {
		return
	}
	run, err := s.app.RunScheduledTaskNow(r.Context(), task.ID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleScheduledTaskRuns(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	task, ok, err := s.authorizedScheduledTask(r, userID, chi.URLParam(r, "taskID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "scheduled task not found")
		return
	}
	limit, err := parseLimit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	runs, err := s.app.ScheduledTaskRuns(r.Context(), task.ID, limit)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) authorizedScheduledTask(r *http.Request, userID string, taskID string) (domain.ScheduledTask, bool, error) {
	task, err := s.app.ScheduledTask(r.Context(), taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ScheduledTask{}, false, nil
		}
		return domain.ScheduledTask{}, false, err
	}
	if authorized, err := s.authorizedOrganization(r, userID, task.OrganizationID); err != nil || !authorized {
		return domain.ScheduledTask{}, authorized, err
	}
	return task, true, nil
}

func (s *Server) authorizedShellTaskManagement(w http.ResponseWriter, r *http.Request, userID string, orgID string) bool {
	role, ok, err := s.authorizedOrganizationRole(r, userID, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return false
	}
	if !ok {
		writeError(w, http.StatusNotFound, "organization not found")
		return false
	}
	if role != domain.RoleOwner && role != domain.RoleAdmin {
		writeError(w, http.StatusForbidden, "admin role required")
		return false
	}
	return true
}

func parseScheduledTaskKind(value string) (domain.ScheduledTaskKind, bool) {
	switch domain.ScheduledTaskKind(value) {
	case domain.ScheduledTaskKindAgentPrompt:
		return domain.ScheduledTaskKindAgentPrompt, true
	case domain.ScheduledTaskKindShellCommand:
		return domain.ScheduledTaskKindShellCommand, true
	default:
		return "", false
	}
}

func parseOptionalConversationType(value string) (domain.ConversationType, bool) {
	if value == "" {
		return "", true
	}
	return parseConversationType(value)
}
