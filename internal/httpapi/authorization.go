package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/meteorsky/agentx/internal/domain"
)

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

func (s *Server) authorizedOrganizationRole(r *http.Request, userID string, orgID string) (domain.Role, bool, error) {
	role, err := s.app.OrganizationRole(r.Context(), orgID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return role, true, nil
}
