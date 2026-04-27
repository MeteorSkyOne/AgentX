package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/app"
)

type namedRequest struct {
	Name string `json:"name"`
}

type projectUpdateRequest struct {
	Name          *string `json:"name"`
	WorkspacePath *string `json:"workspace_path"`
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
