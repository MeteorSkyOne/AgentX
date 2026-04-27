package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/meteorsky/agentx/internal/app"
	"github.com/meteorsky/agentx/internal/domain"
)

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

func (s *Server) handleAgentChannels(w http.ResponseWriter, r *http.Request) {
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
	channels, err := s.app.AgentChannels(r.Context(), agent.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, channels)
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
