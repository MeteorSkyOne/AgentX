package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/meteorsky/agentx/internal/domain"
	agentruntime "github.com/meteorsky/agentx/internal/runtime"
)

// Background tasks are the work agents leave running outside their replies —
// shell commands, monitors and subagents. Runtimes report each session's task
// list as it changes; the app keeps the latest list in memory, publishes it to
// the conversation, and lets users read a task's output or stop it.

// backgroundTaskOutputLimit is how much of the end of a task's output is shown.
const backgroundTaskOutputLimit = 64 * 1024

var ErrBackgroundTaskNotFound = errors.New("background task not found")

// BackgroundTaskOutput is the end of a background task's output.
type BackgroundTaskOutput struct {
	Output string `json:"output"`
	// Truncated reports that earlier output was left out.
	Truncated bool `json:"truncated"`
}

func (a *App) registerBackgroundTaskHandlers() {
	for _, rt := range a.opts.Runtimes {
		if reporter, ok := rt.(agentruntime.BackgroundTaskReporter); ok {
			reporter.SetBackgroundTaskHandler(a.handleBackgroundTasks)
		}
	}
}

func (a *App) handleBackgroundTasks(sessionKey string, tasks []agentruntime.BackgroundTask) {
	agentID, conversationType, conversationID, ok := parseAgentSessionKey(sessionKey)
	if !ok {
		return
	}
	a.backgroundTasksMu.Lock()
	a.backgroundTasks[sessionKey] = append([]agentruntime.BackgroundTask(nil), tasks...)
	a.backgroundTasksMu.Unlock()

	// Publishing looks up the conversation, so it runs off the runtime's
	// goroutine. It sends the list as it is then, so publishes that race each
	// other still end on the latest one.
	a.startBackground("publish background tasks", func(ctx context.Context) {
		scope, err := a.conversationScope(ctx, conversationType, conversationID)
		if err != nil {
			slog.Warn("background tasks not published: conversation lookup failed",
				"agent_id", agentID, "conversation_type", conversationType, "conversation_id", conversationID, "error", err)
			return
		}
		a.publishConversationEvent(domain.Event{
			Type:             domain.EventAgentBackgroundTasksUpdated,
			OrganizationID:   scope.organizationID,
			ConversationType: conversationType,
			ConversationID:   conversationID,
			Payload: domain.AgentBackgroundTasksPayload{
				AgentID: agentID,
				Tasks:   a.agentBackgroundTasks(agentID, sessionKey),
			},
		})
	})
}

func (a *App) agentBackgroundTasks(agentID string, sessionKey string) []domain.BackgroundTask {
	a.backgroundTasksMu.Lock()
	defer a.backgroundTasksMu.Unlock()
	tasks := make([]domain.BackgroundTask, 0, len(a.backgroundTasks[sessionKey]))
	for _, task := range a.backgroundTasks[sessionKey] {
		tasks = append(tasks, backgroundTaskToDomain(agentID, task))
	}
	return tasks
}

// ListBackgroundTasks lists the background tasks of every agent in a
// conversation, oldest first.
func (a *App) ListBackgroundTasks(conversationType domain.ConversationType, conversationID string) []domain.BackgroundTask {
	a.backgroundTasksMu.Lock()
	var tasks []domain.BackgroundTask
	for key, sessionTasks := range a.backgroundTasks {
		agentID, keyType, keyID, ok := parseAgentSessionKey(key)
		if !ok || keyType != conversationType || keyID != conversationID {
			continue
		}
		for _, task := range sessionTasks {
			tasks = append(tasks, backgroundTaskToDomain(agentID, task))
		}
	}
	a.backgroundTasksMu.Unlock()
	sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].StartedAt.Before(tasks[j].StartedAt) })
	if tasks == nil {
		tasks = []domain.BackgroundTask{}
	}
	return tasks
}

func (a *App) backgroundTask(agentID string, conversationType domain.ConversationType, conversationID string, taskID string) (agentruntime.BackgroundTask, bool) {
	a.backgroundTasksMu.Lock()
	defer a.backgroundTasksMu.Unlock()
	for _, task := range a.backgroundTasks[agentSessionKey(agentID, conversationType, conversationID)] {
		if task.ID == taskID {
			return task, true
		}
	}
	return agentruntime.BackgroundTask{}, false
}

// ReadBackgroundTaskOutput returns the end of a background task's output.
func (a *App) ReadBackgroundTaskOutput(conversationType domain.ConversationType, conversationID string, agentID string, taskID string) (BackgroundTaskOutput, error) {
	task, ok := a.backgroundTask(agentID, conversationType, conversationID, taskID)
	if !ok || task.OutputFile == "" {
		return BackgroundTaskOutput{}, ErrBackgroundTaskNotFound
	}
	file, err := os.Open(task.OutputFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Claude Code cleans task output up after a while.
			return BackgroundTaskOutput{}, nil
		}
		return BackgroundTaskOutput{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return BackgroundTaskOutput{}, err
	}
	var offset int64
	if info.Size() > backgroundTaskOutputLimit {
		offset = info.Size() - backgroundTaskOutputLimit
	}
	data, err := io.ReadAll(io.NewSectionReader(file, offset, backgroundTaskOutputLimit))
	if err != nil {
		return BackgroundTaskOutput{}, err
	}
	text := string(data)
	if offset > 0 {
		// Start on a whole line rather than mid-line or mid-rune.
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
	}
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "�")
	}
	return BackgroundTaskOutput{Output: text, Truncated: offset > 0}, nil
}

// StopBackgroundTask stops a running background task.
func (a *App) StopBackgroundTask(ctx context.Context, conversationType domain.ConversationType, conversationID string, agentID string, taskID string) error {
	task, ok := a.backgroundTask(agentID, conversationType, conversationID, taskID)
	if !ok || task.Status != agentruntime.BackgroundTaskRunning {
		return ErrBackgroundTaskNotFound
	}
	agent, err := a.store.Agents().ByID(ctx, agentID)
	if err != nil {
		return err
	}
	rt, ok := a.runtimeForAgent(agent)
	if !ok {
		return ErrBackgroundTaskNotFound
	}
	reporter, ok := rt.(agentruntime.BackgroundTaskReporter)
	if !ok {
		return ErrBackgroundTaskNotFound
	}
	return reporter.StopBackgroundTask(ctx, agentSessionKey(agentID, conversationType, conversationID), taskID)
}

func backgroundTaskToDomain(agentID string, task agentruntime.BackgroundTask) domain.BackgroundTask {
	result := domain.BackgroundTask{
		ID:          task.ID,
		AgentID:     agentID,
		Kind:        task.Kind,
		Description: task.Description,
		ToolCallID:  task.ToolCallID,
		Status:      task.Status,
		Summary:     task.Summary,
		HasOutput:   task.OutputFile != "",
		StartedAt:   task.StartedAt,
	}
	if !task.EndedAt.IsZero() {
		endedAt := task.EndedAt
		result.EndedAt = &endedAt
	}
	return result
}
