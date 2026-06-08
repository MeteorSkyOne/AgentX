package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
)

type roadmapStageRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type roadmapStageUpdateRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
}

type roadmapTaskRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type roadmapTaskUpdateRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Completed   *bool   `json:"completed"`
}

type reorderRequest struct {
	IDs []string `json:"ids"`
}

func (s *Server) handleProjectRoadmap(w http.ResponseWriter, r *http.Request) {
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
	roadmap, err := s.app.ProjectRoadmap(r.Context(), project.ID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, roadmap)
}

func (s *Server) handleCreateRoadmapStage(w http.ResponseWriter, r *http.Request) {
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
	var req roadmapStageRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	stage, err := s.app.CreateRoadmapStage(r.Context(), app.RoadmapStageCreateRequest{
		ProjectID:   project.ID,
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stage)
}

func (s *Server) handleUpdateRoadmapStage(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	stage, ok, err := s.authorizedRoadmapStage(r, userID, chi.URLParam(r, "stageID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "roadmap stage not found")
		return
	}
	var req roadmapStageUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	updated, err := s.app.UpdateRoadmapStage(r.Context(), stage.ID, app.RoadmapStageUpdateRequest{
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteRoadmapStage(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	_, ok, err := s.authorizedRoadmapStage(r, userID, chi.URLParam(r, "stageID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "roadmap stage not found")
		return
	}
	if err := s.app.DeleteRoadmapStage(r.Context(), chi.URLParam(r, "stageID")); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReorderRoadmapStages(w http.ResponseWriter, r *http.Request) {
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
	var req reorderRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	if err := s.app.ReorderRoadmapStages(r.Context(), project.ID, req.IDs); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreateRoadmapTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	stage, ok, err := s.authorizedRoadmapStage(r, userID, chi.URLParam(r, "stageID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "roadmap stage not found")
		return
	}
	var req roadmapTaskRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	task, err := s.app.CreateRoadmapTask(r.Context(), app.RoadmapTaskCreateRequest{
		StageID:     stage.ID,
		Title:       req.Title,
		Description: req.Description,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleUpdateRoadmapTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	task, ok, err := s.authorizedRoadmapTask(r, userID, chi.URLParam(r, "taskID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "roadmap task not found")
		return
	}
	var req roadmapTaskUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	updated, err := s.app.UpdateRoadmapTask(r.Context(), task.ID, app.RoadmapTaskUpdateRequest{
		Title:       req.Title,
		Description: req.Description,
		Completed:   req.Completed,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteRoadmapTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	_, ok, err := s.authorizedRoadmapTask(r, userID, chi.URLParam(r, "taskID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "roadmap task not found")
		return
	}
	if err := s.app.DeleteRoadmapTask(r.Context(), chi.URLParam(r, "taskID")); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReorderRoadmapTasks(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	stage, ok, err := s.authorizedRoadmapStage(r, userID, chi.URLParam(r, "stageID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "roadmap stage not found")
		return
	}
	var req reorderRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON")
		return
	}
	if err := s.app.ReorderRoadmapTasks(r.Context(), stage.ID, req.IDs); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) authorizedRoadmapStage(r *http.Request, userID string, stageID string) (domain.RoadmapStage, bool, error) {
	stage, err := s.app.RoadmapStage(r.Context(), stageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.RoadmapStage{}, false, nil
		}
		return domain.RoadmapStage{}, false, err
	}
	if authorized, err := s.authorizedOrganization(r, userID, stage.OrganizationID); err != nil || !authorized {
		return domain.RoadmapStage{}, authorized, err
	}
	return stage, true, nil
}

func (s *Server) authorizedRoadmapTask(r *http.Request, userID string, taskID string) (domain.RoadmapTask, bool, error) {
	task, err := s.app.RoadmapTask(r.Context(), taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.RoadmapTask{}, false, nil
		}
		return domain.RoadmapTask{}, false, err
	}
	if authorized, err := s.authorizedOrganization(r, userID, task.OrganizationID); err != nil || !authorized {
		return domain.RoadmapTask{}, authorized, err
	}
	return task, true, nil
}
