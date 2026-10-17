package app

import (
	"context"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/eventbus"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
)

func TestRunProgressIsPublishedAndReplayed(t *testing.T) {
	ctx := context.Background()
	app, bus, bootstrap := newConversationTestApp(t, ctx)
	tokens := int64(1200)
	app.opts.Runtimes[domain.AgentKindFake] = scriptedRuntime{events: []agentruntime.Event{
		{Type: agentruntime.EventDelta, Goal: &agentruntime.GoalState{Objective: "ship it", Status: agentruntime.GoalStatusActive, TokensUsed: &tokens}},
		{Type: agentruntime.EventDelta, Plan: &agentruntime.PlanState{Steps: []agentruntime.PlanStep{
			{Step: "Inspect", Status: agentruntime.PlanStepCompleted},
			{Step: "Patch", Status: agentruntime.PlanStepInProgress},
		}}},
		{
			Type:         agentruntime.EventInputRequest,
			InputRequest: &agentruntime.InputRequest{QuestionID: "question-1", Question: "continue?"},
		},
	}}

	events, unsubscribe := bus.Subscribe(ctx, eventbus.Filter{
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
	})
	defer unsubscribe()

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "work on the goal",
	}); err != nil {
		t.Fatal(err)
	}

	var runID string
	var progress []domain.AgentRunProgressPayload
	timeout := time.After(2 * time.Second)
	for runID == "" {
		select {
		case evt := <-events:
			switch evt.Type {
			case domain.EventAgentRunProgress:
				progress = append(progress, evt.Payload.(domain.AgentRunProgressPayload))
			case domain.EventAgentInputRequest:
				runID = evt.Payload.(domain.AgentInputRequestPayload).RunID
			}
		case <-timeout:
			t.Fatal("timed out waiting for input request")
		}
	}
	if len(progress) != 2 || progress[0].Goal == nil || progress[0].Plan != nil {
		t.Fatalf("progress events = %#v", progress)
	}
	// Each progress event carries the run's full state, so a plan update keeps the goal.
	if progress[1].Goal == nil || progress[1].Plan == nil || len(progress[1].Plan.Steps) != 2 || progress[1].RunID != runID {
		t.Fatalf("second progress = %#v", progress[1])
	}

	replay := app.ActiveRunReplayEvents(bootstrap.Organization.ID, domain.ConversationChannel, bootstrap.Channel.ID)[runID]
	var replayed *domain.AgentRunProgressPayload
	for _, evt := range replay.Events {
		if evt.Type == domain.EventAgentRunProgress {
			payload := evt.Payload.(domain.AgentRunProgressPayload)
			replayed = &payload
		}
	}
	if replayed == nil || replayed.Goal.Objective != "ship it" || replayed.Plan.Steps[1].Status != agentruntime.PlanStepInProgress {
		t.Fatalf("replay = %#v", replay.Events)
	}

}

func TestRunProgressIsPersistedInMessageMetadata(t *testing.T) {
	ctx := context.Background()
	app, bus, bootstrap := newConversationTestApp(t, ctx)
	app.opts.Runtimes[domain.AgentKindFake] = scriptedRuntime{events: []agentruntime.Event{
		{Type: agentruntime.EventDelta, Goal: &agentruntime.GoalState{Objective: "ship it", Status: agentruntime.GoalStatusActive}},
		{Type: agentruntime.EventDelta, Plan: &agentruntime.PlanState{Steps: []agentruntime.PlanStep{
			{Step: "Inspect", Status: agentruntime.PlanStepCompleted},
			{Step: "Patch", Status: agentruntime.PlanStepCompleted},
		}}},
		{Type: agentruntime.EventDelta, Goal: &agentruntime.GoalState{Objective: "ship it", Status: agentruntime.GoalStatusComplete}},
		{Type: agentruntime.EventCompleted, Text: "done"},
	}}

	events, unsubscribe := bus.Subscribe(ctx, eventbus.Filter{
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
	})
	defer unsubscribe()

	if _, err := app.SendMessage(ctx, SendMessageRequest{
		UserID:           bootstrap.User.ID,
		OrganizationID:   bootstrap.Organization.ID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   bootstrap.Channel.ID,
		Body:             "work on the goal",
	}); err != nil {
		t.Fatal(err)
	}

	timeout := time.After(2 * time.Second)
	var botMessage domain.Message
	for botMessage.ID == "" {
		select {
		case evt := <-events:
			if evt.Type != domain.EventMessageCreated {
				continue
			}
			if message := evt.Payload.(domain.MessageCreatedPayload).Message; message.SenderType == domain.SenderBot {
				botMessage = message
			}
		case <-timeout:
			t.Fatal("timed out waiting for bot message")
		}
	}
	goal, ok := botMessage.Metadata["goal"].(*domain.AgentGoal)
	if !ok || goal.Status != agentruntime.GoalStatusComplete {
		t.Fatalf("goal metadata = %#v", botMessage.Metadata["goal"])
	}
	plan, ok := botMessage.Metadata["plan"].(*domain.AgentPlan)
	if !ok || len(plan.Steps) != 2 {
		t.Fatalf("plan metadata = %#v", botMessage.Metadata["plan"])
	}

	messages, err := app.ListMessages(ctx, domain.ConversationChannel, bootstrap.Channel.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		if message.ID != botMessage.ID {
			continue
		}
		stored, ok := message.Metadata["goal"].(map[string]any)
		if !ok || stored["objective"] != "ship it" || stored["status"] != "complete" {
			t.Fatalf("stored goal = %#v", message.Metadata["goal"])
		}
		return
	}
	t.Fatalf("bot message %q not stored", botMessage.ID)
}
