package app

import (
	"context"
	"errors"
	"testing"

	"github.com/meteorsky/agentx/internal/domain"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
)

type stubSession struct{}

func (stubSession) Send(context.Context, agentruntime.Input) error { return nil }
func (stubSession) Events() <-chan agentruntime.Event              { return nil }
func (stubSession) CurrentSessionID() string                       { return "" }
func (stubSession) Alive() bool                                    { return true }
func (stubSession) Close(context.Context) error                    { return nil }
func (stubSession) RespondToInputRequest(string, string) error     { return nil }

type stubSubagentStopper struct {
	stubSession
	stopped []string
	err     error
}

func (s *stubSubagentStopper) StopSubagent(_ context.Context, toolCallID string) error {
	s.stopped = append(s.stopped, toolCallID)
	return s.err
}

func appWithRun(key activeRunKey, run *activeAgentRun) *App {
	return &App{activeRuns: map[activeRunKey]map[string]*activeAgentRun{
		key: {run.runID: run},
	}}
}

func TestStopSubagentRoutesToSupportingSession(t *testing.T) {
	stopper := &stubSubagentStopper{}
	key := activeRunKey{conversationType: domain.ConversationChannel, conversationID: "chan-1", agentID: "agent-1"}
	a := appWithRun(key, &activeAgentRun{
		runID:            "run-1",
		organizationID:   "org-1",
		conversationType: key.conversationType,
		conversationID:   key.conversationID,
		session:          stopper,
	})

	if err := a.StopSubagent(context.Background(), "org-1", key.conversationType, "chan-1", "toolu_x"); err != nil {
		t.Fatalf("StopSubagent: %v", err)
	}
	if len(stopper.stopped) != 1 || stopper.stopped[0] != "toolu_x" {
		t.Errorf("expected the session to receive the stop, got %#v", stopper.stopped)
	}
}

func TestStopSubagentRejectsWrongConversationOrOrg(t *testing.T) {
	stopper := &stubSubagentStopper{}
	key := activeRunKey{conversationType: domain.ConversationChannel, conversationID: "chan-1", agentID: "agent-1"}
	a := appWithRun(key, &activeAgentRun{
		runID:            "run-1",
		organizationID:   "org-1",
		conversationType: key.conversationType,
		conversationID:   key.conversationID,
		session:          stopper,
	})

	if err := a.StopSubagent(context.Background(), "org-1", key.conversationType, "chan-other", "toolu_x"); err == nil {
		t.Error("expected an error for a different conversation")
	}
	if err := a.StopSubagent(context.Background(), "org-other", key.conversationType, "chan-1", "toolu_x"); err == nil {
		t.Error("expected an error for a different organization")
	}
	if len(stopper.stopped) != 0 {
		t.Errorf("no stop should have been routed, got %#v", stopper.stopped)
	}
}

func TestStopSubagentReportsUnsupportedSessions(t *testing.T) {
	key := activeRunKey{conversationType: domain.ConversationChannel, conversationID: "chan-1", agentID: "agent-1"}
	a := appWithRun(key, &activeAgentRun{
		runID:            "run-1",
		organizationID:   "org-1",
		conversationType: key.conversationType,
		conversationID:   key.conversationID,
		session:          stubSession{},
	})

	err := a.StopSubagent(context.Background(), "org-1", key.conversationType, "chan-1", "toolu_x")
	if err == nil {
		t.Fatal("expected an error when no session supports stopping subagents")
	}
}

func TestStopSubagentSurfacesSessionError(t *testing.T) {
	stopper := &stubSubagentStopper{err: errors.New("no running subagent for this tool call")}
	key := activeRunKey{conversationType: domain.ConversationChannel, conversationID: "chan-1", agentID: "agent-1"}
	a := appWithRun(key, &activeAgentRun{
		runID:            "run-1",
		organizationID:   "org-1",
		conversationType: key.conversationType,
		conversationID:   key.conversationID,
		session:          stopper,
	})

	if err := a.StopSubagent(context.Background(), "org-1", key.conversationType, "chan-1", "toolu_x"); err == nil {
		t.Fatal("expected the session error to surface")
	}
}
