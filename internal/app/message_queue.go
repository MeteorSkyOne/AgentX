package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
)

var ErrQueuedPromptNotFound = errors.New("queued prompt not found")
var ErrQueuedPromptNotSteerable = errors.New("queued prompt is not steerable")

type queuedAgentPrompt struct {
	QueueID   string
	Key       messageQueueKey
	Message   domain.Message
	Target    ConversationAgentContext
	CreatedAt time.Time
	Steering  bool
}

func (a *App) dispatchAgentRunOrQueue(ctx context.Context, message domain.Message, target ConversationAgentContext) {
	activeKey := activeRunKey{
		conversationType: message.ConversationType,
		conversationID:   message.ConversationID,
		agentID:          target.Agent.ID,
	}
	if a.hasActiveAgentRun(activeKey) {
		a.enqueueAgentPrompt(message, target)
		return
	}
	go a.runAgentForMessage(ctx, message, target)
}

func (a *App) hasActiveAgentRun(key activeRunKey) bool {
	a.activeRunsMu.Lock()
	defer a.activeRunsMu.Unlock()
	return len(a.activeRuns[key]) > 0
}

func (a *App) enqueueAgentPrompt(message domain.Message, target ConversationAgentContext) {
	key := messageQueueKey{
		conversationType: message.ConversationType,
		conversationID:   message.ConversationID,
		agentID:          target.Agent.ID,
	}
	item := &queuedAgentPrompt{
		QueueID:   id.New("queue"),
		Key:       key,
		Message:   message,
		Target:    target,
		CreatedAt: time.Now().UTC(),
	}

	a.messageQueueMu.Lock()
	a.messageQueue[key] = append(a.messageQueue[key], item)
	a.messageQueueByID[item.QueueID] = item
	a.messageQueueMu.Unlock()

	a.publishAgentPromptQueued(item)
}

func (a *App) handleAgentRunTerminated(ctx context.Context, key activeRunKey, status string) {
	if a.hasActiveAgentRun(key) {
		return
	}
	queueKey := messageQueueKeyFromActiveRunKey(key)
	switch status {
	case "completed", "canceled":
		a.startNextQueuedPrompt(ctx, queueKey)
	default:
		a.clearQueuedAgentPrompts(queueKey, "failed")
	}
}

func (a *App) startNextQueuedPrompt(ctx context.Context, key messageQueueKey) bool {
	if a.hasActiveAgentRun(activeRunKeyFromMessageQueueKey(key)) {
		return false
	}
	item, ok := a.dequeueNextQueuedPrompt(key, "started")
	if !ok {
		return false
	}
	go a.runAgentForMessage(ctx, item.Message, item.Target)
	return true
}

func (a *App) dequeueNextQueuedPrompt(key messageQueueKey, status string) (*queuedAgentPrompt, bool) {
	a.messageQueueMu.Lock()
	items := a.messageQueue[key]
	if len(items) == 0 || items[0].Steering {
		a.messageQueueMu.Unlock()
		return nil, false
	}
	item := items[0]
	if len(items) == 1 {
		delete(a.messageQueue, key)
	} else {
		a.messageQueue[key] = items[1:]
	}
	delete(a.messageQueueByID, item.QueueID)
	a.messageQueueMu.Unlock()

	a.publishAgentPromptQueueRemoved(item, status)
	return item, true
}

func (a *App) clearQueuedAgentPrompts(key messageQueueKey, status string) int {
	a.messageQueueMu.Lock()
	items := append([]*queuedAgentPrompt(nil), a.messageQueue[key]...)
	delete(a.messageQueue, key)
	for _, item := range items {
		delete(a.messageQueueByID, item.QueueID)
	}
	a.messageQueueMu.Unlock()

	for _, item := range items {
		a.publishAgentPromptQueueRemoved(item, status)
	}
	return len(items)
}

func (a *App) SteerQueuedPrompt(ctx context.Context, conversationType domain.ConversationType, conversationID string, queueID string) error {
	queueID = strings.TrimSpace(queueID)
	if queueID == "" {
		return ErrQueuedPromptNotFound
	}

	item, err := a.markQueuedPromptSteering(conversationType, conversationID, queueID)
	if err != nil {
		return err
	}
	clearSteering := true
	defer func() {
		if clearSteering {
			a.unmarkQueuedPromptSteering(queueID)
		}
	}()

	if item.Target.Agent.Kind != domain.AgentKindCodexPersistent || len(item.Message.Attachments) > 0 {
		return ErrQueuedPromptNotSteerable
	}
	steerer := a.activeSteererForKey(activeRunKeyFromMessageQueueKey(item.Key))
	if steerer == nil {
		return ErrQueuedPromptNotSteerable
	}

	prompt, err := a.promptWithReplyReference(ctx, item.Message, item.Message.Body)
	if err != nil {
		return err
	}
	if err := steerer.Steer(ctx, agentruntime.Input{Prompt: prompt}); err != nil {
		clearSteering = false
		a.unmarkQueuedPromptSteering(queueID)
		if !a.hasActiveAgentRun(activeRunKeyFromMessageQueueKey(item.Key)) {
			a.startNextQueuedPrompt(context.WithoutCancel(ctx), item.Key)
		}
		return err
	}

	clearSteering = false
	if !a.removeQueuedPromptByID(queueID, "steered") {
		return ErrQueuedPromptNotFound
	}
	return nil
}

func (a *App) markQueuedPromptSteering(conversationType domain.ConversationType, conversationID string, queueID string) (*queuedAgentPrompt, error) {
	a.messageQueueMu.Lock()
	defer a.messageQueueMu.Unlock()
	item := a.messageQueueByID[queueID]
	if item == nil || item.Key.conversationType != conversationType || item.Key.conversationID != conversationID {
		return nil, ErrQueuedPromptNotFound
	}
	if item.Steering {
		return nil, ErrQueuedPromptNotSteerable
	}
	item.Steering = true
	return item, nil
}

func (a *App) unmarkQueuedPromptSteering(queueID string) {
	a.messageQueueMu.Lock()
	if item := a.messageQueueByID[queueID]; item != nil {
		item.Steering = false
	}
	a.messageQueueMu.Unlock()
}

func (a *App) removeQueuedPromptByID(queueID string, status string) bool {
	a.messageQueueMu.Lock()
	item := a.messageQueueByID[queueID]
	if item == nil {
		a.messageQueueMu.Unlock()
		return false
	}
	items := a.messageQueue[item.Key]
	for index, candidate := range items {
		if candidate.QueueID != queueID {
			continue
		}
		items = append(items[:index], items[index+1:]...)
		break
	}
	if len(items) == 0 {
		delete(a.messageQueue, item.Key)
	} else {
		a.messageQueue[item.Key] = items
	}
	delete(a.messageQueueByID, queueID)
	a.messageQueueMu.Unlock()

	a.publishAgentPromptQueueRemoved(item, status)
	return true
}

func (a *App) activeSteererForKey(key activeRunKey) agentruntime.Steerer {
	a.activeRunsMu.Lock()
	runs := make([]*activeAgentRun, 0, len(a.activeRuns[key]))
	for _, run := range a.activeRuns[key] {
		runs = append(runs, run)
	}
	a.activeRunsMu.Unlock()

	for _, run := range runs {
		run.mu.Lock()
		session := run.session
		run.mu.Unlock()
		if steerer, ok := session.(agentruntime.Steerer); ok {
			return steerer
		}
	}
	return nil
}

func (a *App) PromptQueueReplayEvents(organizationID string, conversationType domain.ConversationType, conversationID string) map[string]ActiveRunReplay {
	a.messageQueueMu.Lock()
	items := make([]*queuedAgentPrompt, 0)
	for key, queued := range a.messageQueue {
		if key.conversationType != conversationType || key.conversationID != conversationID {
			continue
		}
		for _, item := range queued {
			if item.Message.OrganizationID == organizationID {
				items = append(items, item)
			}
		}
	}
	a.messageQueueMu.Unlock()

	captured := time.Now().UTC()
	replays := make(map[string]ActiveRunReplay, len(items))
	for _, item := range items {
		replays[item.QueueID] = ActiveRunReplay{
			Events: []domain.Event{{
				Type:             domain.EventAgentPromptQueued,
				OrganizationID:   item.Message.OrganizationID,
				ConversationType: item.Message.ConversationType,
				ConversationID:   item.Message.ConversationID,
				Payload:          a.agentPromptQueuedPayload(item),
				CreatedAt:        item.CreatedAt,
			}},
			Captured: captured,
		}
	}
	return replays
}

func (a *App) publishAgentPromptQueued(item *queuedAgentPrompt) {
	a.publishConversationEvent(domain.Event{
		Type:             domain.EventAgentPromptQueued,
		OrganizationID:   item.Message.OrganizationID,
		ConversationType: item.Message.ConversationType,
		ConversationID:   item.Message.ConversationID,
		Payload:          a.agentPromptQueuedPayload(item),
	})
}

func (a *App) publishAgentPromptQueueRemoved(item *queuedAgentPrompt, status string) {
	a.publishConversationEvent(domain.Event{
		Type:             domain.EventAgentPromptQueueRemoved,
		OrganizationID:   item.Message.OrganizationID,
		ConversationType: item.Message.ConversationType,
		ConversationID:   item.Message.ConversationID,
		Payload:          domain.AgentPromptQueueRemovedPayload{QueueID: item.QueueID, Status: status},
	})
}

func (a *App) agentPromptQueuedPayload(item *queuedAgentPrompt) domain.AgentPromptQueuedPayload {
	return domain.AgentPromptQueuedPayload{
		QueueID:   item.QueueID,
		MessageID: item.Message.ID,
		AgentID:   item.Target.Agent.ID,
		Body:      item.Message.Body,
		CreatedAt: item.CreatedAt,
		CanSteer:  a.queuedPromptCanSteer(item),
	}
}

func (a *App) queuedPromptCanSteer(item *queuedAgentPrompt) bool {
	if item.Target.Agent.Kind != domain.AgentKindCodexPersistent || len(item.Message.Attachments) > 0 {
		return false
	}
	steerer := a.activeSteererForKey(activeRunKeyFromMessageQueueKey(item.Key))
	if steerer == nil {
		return false
	}
	if ready, ok := steerer.(interface{ CanSteer() bool }); ok {
		return ready.CanSteer()
	}
	return true
}

func messageQueueKeyFromActiveRunKey(key activeRunKey) messageQueueKey {
	return messageQueueKey{
		conversationType: key.conversationType,
		conversationID:   key.conversationID,
		agentID:          key.agentID,
	}
}

func activeRunKeyFromMessageQueueKey(key messageQueueKey) activeRunKey {
	return activeRunKey{
		conversationType: key.conversationType,
		conversationID:   key.conversationID,
		agentID:          key.agentID,
	}
}
