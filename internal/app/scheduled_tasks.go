package app

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
	"github.com/robfig/cron/v3"
)

const (
	defaultScheduledTaskTimezone       = "UTC"
	defaultScheduledTaskTimeoutSeconds = 600
	maxScheduledTaskTimeoutSeconds     = 86400
	scheduledTaskOutputLimit           = 64 * 1024
	defaultScheduledPostTitleTemplate  = "${task} · ${datetime}"
	maxScheduledPostTitleLength        = 200
)

type ScheduledTaskCreateRequest struct {
	UserID           string
	ProjectID        string
	Name             string
	Kind             domain.ScheduledTaskKind
	Enabled          bool
	Schedule         string
	Timezone         string
	ConversationType domain.ConversationType
	ConversationID   string
	AgentID          string
	WorkspaceID      string
	Prompt           string
	Command          string
	PostTitle        string
	FreshContext     bool
	// Notify defaults to true when nil so callers that omit it keep the
	// historical behavior of notifying on every scheduled agent reply.
	Notify         *bool
	TimeoutSeconds int
}

type ScheduledTaskUpdateRequest struct {
	Name             *string
	Kind             *domain.ScheduledTaskKind
	Enabled          *bool
	Schedule         *string
	Timezone         *string
	ConversationType *domain.ConversationType
	ConversationID   *string
	AgentID          *string
	WorkspaceID      *string
	Prompt           *string
	Command          *string
	PostTitle        *string
	FreshContext     *bool
	Notify           *bool
	TimeoutSeconds   *int
}

func (a *App) StartScheduledTasks(ctx context.Context) error {
	if a.scheduledTasks != nil {
		return nil
	}
	scheduler := newScheduledTaskScheduler(a)
	tasks, err := a.store.ScheduledTasks().ListEnabled(ctx)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if err := scheduler.upsert(ctx, task); err != nil {
			return err
		}
	}
	scheduler.start()
	a.scheduledTasks = scheduler
	return nil
}

func (a *App) StopScheduledTasks() {
	if a.scheduledTasks == nil {
		return
	}
	a.scheduledTasks.stop()
	a.scheduledTasks = nil
}

func (a *App) ListScheduledTasks(ctx context.Context, projectID string) ([]domain.ScheduledTask, error) {
	return a.store.ScheduledTasks().ListByProject(ctx, projectID)
}

func (a *App) ScheduledTask(ctx context.Context, id string) (domain.ScheduledTask, error) {
	return a.store.ScheduledTasks().ByID(ctx, id)
}

func (a *App) ScheduledTaskRuns(ctx context.Context, taskID string, limit int) ([]domain.ScheduledTaskRun, error) {
	return a.store.ScheduledTasks().ListRunsByTask(ctx, taskID, limit)
}

func (a *App) CreateScheduledTask(ctx context.Context, req ScheduledTaskCreateRequest) (domain.ScheduledTask, error) {
	project, err := a.store.Projects().ByID(ctx, req.ProjectID)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	now := time.Now().UTC()
	task := domain.ScheduledTask{
		ID:               id.New("tsk"),
		OrganizationID:   project.OrganizationID,
		ProjectID:        project.ID,
		Name:             req.Name,
		Kind:             req.Kind,
		Enabled:          req.Enabled,
		Schedule:         req.Schedule,
		Timezone:         req.Timezone,
		ConversationType: req.ConversationType,
		ConversationID:   req.ConversationID,
		AgentID:          req.AgentID,
		WorkspaceID:      req.WorkspaceID,
		Prompt:           req.Prompt,
		Command:          req.Command,
		PostTitle:        req.PostTitle,
		FreshContext:     req.FreshContext,
		Notify:           req.Notify == nil || *req.Notify,
		TimeoutSeconds:   req.TimeoutSeconds,
		CreatedBy:        req.UserID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	task, err = a.normalizeScheduledTask(ctx, project, task)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	if err := a.store.ScheduledTasks().Create(ctx, task); err != nil {
		return domain.ScheduledTask{}, err
	}
	if a.scheduledTasks != nil {
		if err := a.scheduledTasks.upsert(ctx, task); err != nil {
			return domain.ScheduledTask{}, err
		}
	}
	return task, nil
}

func (a *App) UpdateScheduledTask(ctx context.Context, taskID string, req ScheduledTaskUpdateRequest) (domain.ScheduledTask, error) {
	task, err := a.store.ScheduledTasks().ByID(ctx, taskID)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	project, err := a.store.Projects().ByID(ctx, task.ProjectID)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	if req.Name != nil {
		task.Name = *req.Name
	}
	if req.Kind != nil {
		task.Kind = *req.Kind
	}
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}
	if req.Schedule != nil {
		task.Schedule = *req.Schedule
	}
	if req.Timezone != nil {
		task.Timezone = *req.Timezone
	}
	if req.ConversationType != nil {
		task.ConversationType = *req.ConversationType
	}
	if req.ConversationID != nil {
		task.ConversationID = *req.ConversationID
	}
	if req.AgentID != nil {
		task.AgentID = *req.AgentID
	}
	if req.WorkspaceID != nil {
		task.WorkspaceID = *req.WorkspaceID
	}
	if req.Prompt != nil {
		task.Prompt = *req.Prompt
	}
	if req.Command != nil {
		task.Command = *req.Command
	}
	if req.PostTitle != nil {
		task.PostTitle = *req.PostTitle
	}
	if req.FreshContext != nil {
		task.FreshContext = *req.FreshContext
	}
	if req.Notify != nil {
		task.Notify = *req.Notify
	}
	if req.TimeoutSeconds != nil {
		task.TimeoutSeconds = *req.TimeoutSeconds
	}
	task.UpdatedAt = time.Now().UTC()
	task, err = a.normalizeScheduledTask(ctx, project, task)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	if err := a.store.ScheduledTasks().Update(ctx, task); err != nil {
		return domain.ScheduledTask{}, err
	}
	if a.scheduledTasks != nil {
		if err := a.scheduledTasks.upsert(ctx, task); err != nil {
			return domain.ScheduledTask{}, err
		}
	}
	return task, nil
}

func (a *App) DeleteScheduledTask(ctx context.Context, taskID string) error {
	if a.scheduledTasks != nil {
		a.scheduledTasks.remove(taskID)
	}
	return a.store.ScheduledTasks().Delete(ctx, taskID)
}

func (a *App) RunScheduledTaskNow(ctx context.Context, taskID string) (domain.ScheduledTaskRun, error) {
	task, err := a.store.ScheduledTasks().ByID(ctx, taskID)
	if err != nil {
		return domain.ScheduledTaskRun{}, err
	}
	run, ok, err := a.beginScheduledTaskRun(ctx, task, domain.ScheduledTaskTriggerManual, nil)
	if err != nil || !ok {
		return run, err
	}
	if !a.startBackground("scheduled-task-run", func(bgCtx context.Context) {
		a.executeScheduledTaskRun(bgCtx, task, run)
	}) {
		a.failScheduledTaskRunStart(ctx, task, run, errAppShuttingDown)
		return run, errAppShuttingDown
	}
	return run, nil
}

func (a *App) normalizeScheduledTask(ctx context.Context, project domain.Project, task domain.ScheduledTask) (domain.ScheduledTask, error) {
	task.Name = strings.TrimSpace(task.Name)
	if task.Name == "" {
		return domain.ScheduledTask{}, invalidInput("task name is required")
	}
	task.Schedule = strings.TrimSpace(task.Schedule)
	if task.Schedule == "" {
		return domain.ScheduledTask{}, invalidInput("schedule is required")
	}
	task.Timezone = strings.TrimSpace(task.Timezone)
	if task.Timezone == "" {
		task.Timezone = defaultScheduledTaskTimezone
	}
	nextRunAt, err := nextScheduledTaskRunAt(task, time.Now().UTC())
	if err != nil {
		return domain.ScheduledTask{}, invalidInput("invalid schedule or timezone")
	}
	task.NextRunAt = &nextRunAt
	if !task.Enabled {
		task.NextRunAt = nil
	}
	if task.TimeoutSeconds <= 0 {
		task.TimeoutSeconds = defaultScheduledTaskTimeoutSeconds
	}
	if task.TimeoutSeconds > maxScheduledTaskTimeoutSeconds {
		return domain.ScheduledTask{}, invalidInput("timeout_seconds must be between 1 and 86400")
	}
	task.WorkspaceID = strings.TrimSpace(task.WorkspaceID)
	if task.WorkspaceID == "" {
		task.WorkspaceID = project.WorkspaceID
	}
	workspace, err := a.store.Workspaces().ByID(ctx, task.WorkspaceID)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	if workspace.OrganizationID != project.OrganizationID {
		return domain.ScheduledTask{}, invalidInput("workspace does not belong to the project organization")
	}

	switch task.Kind {
	case domain.ScheduledTaskKindAgentPrompt:
		return a.normalizeAgentPromptTask(ctx, project, task)
	case domain.ScheduledTaskKindForumPost:
		return a.normalizeForumPostTask(ctx, project, task)
	case domain.ScheduledTaskKindShellCommand:
		return a.normalizeShellCommandTask(task)
	default:
		return domain.ScheduledTask{}, invalidInput("unknown task kind")
	}
}

func (a *App) normalizeAgentPromptTask(ctx context.Context, project domain.Project, task domain.ScheduledTask) (domain.ScheduledTask, error) {
	task.Prompt = strings.TrimSpace(task.Prompt)
	task.Command = ""
	task.PostTitle = ""
	if task.Prompt == "" {
		return domain.ScheduledTask{}, invalidInput("prompt is required")
	}
	task.ConversationID = strings.TrimSpace(task.ConversationID)
	if task.ConversationType == "" || task.ConversationID == "" {
		return domain.ScheduledTask{}, invalidInput("target conversation is required")
	}
	scope, err := a.conversationScope(ctx, task.ConversationType, task.ConversationID)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	if err := validateScheduledTaskScope(project, scope); err != nil {
		return domain.ScheduledTask{}, err
	}
	if scope.channel.ID != "" && scope.thread == nil && scope.channel.Type == domain.ChannelTypeThread {
		return domain.ScheduledTask{}, invalidInput("forum channels require the forum post task kind")
	}
	task.AgentID, err = a.normalizeScheduledTaskAgent(ctx, scope, task.AgentID)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	return task, nil
}

func (a *App) normalizeForumPostTask(ctx context.Context, project domain.Project, task domain.ScheduledTask) (domain.ScheduledTask, error) {
	task.Prompt = strings.TrimSpace(task.Prompt)
	task.Command = ""
	task.PostTitle = strings.TrimSpace(task.PostTitle)
	if task.Prompt == "" {
		return domain.ScheduledTask{}, invalidInput("prompt is required")
	}
	task.ConversationType = domain.ConversationChannel
	task.ConversationID = strings.TrimSpace(task.ConversationID)
	if task.ConversationID == "" {
		return domain.ScheduledTask{}, invalidInput("target forum channel is required")
	}
	scope, err := a.conversationScope(ctx, task.ConversationType, task.ConversationID)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	if err := validateScheduledTaskScope(project, scope); err != nil {
		return domain.ScheduledTask{}, err
	}
	if scope.channel.Type != domain.ChannelTypeThread {
		return domain.ScheduledTask{}, invalidInput("forum post tasks require a forum channel")
	}
	if _, err := renderScheduledPostTitle(task, time.Now().UTC()); err != nil {
		return domain.ScheduledTask{}, err
	}
	task.AgentID, err = a.normalizeScheduledTaskAgent(ctx, scope, task.AgentID)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	return task, nil
}

func (a *App) normalizeShellCommandTask(task domain.ScheduledTask) (domain.ScheduledTask, error) {
	if !a.opts.ScheduledShellEnabled {
		return domain.ScheduledTask{}, invalidInput("scheduled shell tasks are disabled")
	}
	task.Command = strings.TrimSpace(task.Command)
	task.Prompt = ""
	task.PostTitle = ""
	task.FreshContext = false
	// Shell commands never produce an agent reply, so there is nothing to notify about.
	task.Notify = true
	task.ConversationType = ""
	task.ConversationID = ""
	task.AgentID = ""
	if task.Command == "" {
		return domain.ScheduledTask{}, invalidInput("command is required")
	}
	return task, nil
}

func validateScheduledTaskScope(project domain.Project, scope conversationScope) error {
	if scope.organizationID != project.OrganizationID {
		return invalidInput("target conversation must belong to the task organization")
	}
	if scope.project.ID != "" && scope.project.ID != project.ID {
		return invalidInput("target conversation must belong to the task project")
	}
	return nil
}

func (a *App) normalizeScheduledTaskAgent(ctx context.Context, scope conversationScope, agentID string) (string, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return "", nil
	}
	agents, err := a.conversationAgents(ctx, scope)
	if err != nil {
		return "", err
	}
	for _, item := range agents {
		if item.Agent.ID == agentID {
			return agentID, nil
		}
	}
	return "", invalidInput("agent is not bound to the target conversation")
}

func (a *App) runScheduledTask(ctx context.Context, taskID string, trigger domain.ScheduledTaskTrigger, scheduledFor *time.Time) {
	task, err := a.store.ScheduledTasks().ByID(ctx, taskID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("scheduled task lookup failed", "task_id", taskID, "error", err)
		}
		return
	}
	run, ok, err := a.beginScheduledTaskRun(ctx, task, trigger, scheduledFor)
	if err != nil || !ok {
		if err != nil {
			slog.Error("scheduled task run start failed", "task_id", task.ID, "error", err)
		}
		return
	}
	a.executeScheduledTaskRun(ctx, task, run)
}

func (a *App) startScheduledTaskFromCron(taskID string, scheduledFor time.Time) {
	if a.startBackground("scheduled-task-run", func(bgCtx context.Context) {
		a.runScheduledTask(bgCtx, taskID, domain.ScheduledTaskTriggerScheduled, &scheduledFor)
	}) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a.recordScheduledTaskStartRejected(ctx, taskID, domain.ScheduledTaskTriggerScheduled, &scheduledFor, errAppShuttingDown)
}

func (a *App) recordScheduledTaskStartRejected(ctx context.Context, taskID string, trigger domain.ScheduledTaskTrigger, scheduledFor *time.Time, cause error) {
	task, err := a.store.ScheduledTasks().ByID(ctx, taskID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("scheduled task lookup failed after start rejected", "task_id", taskID, "error", err)
		}
		return
	}
	run, ok, err := a.beginScheduledTaskRun(ctx, task, trigger, scheduledFor)
	if err != nil {
		slog.Error("scheduled task rejected run record failed", "task_id", task.ID, "error", err)
		return
	}
	if !ok {
		return
	}
	a.failScheduledTaskRunStart(ctx, task, run, cause)
	slog.Warn("scheduled task start rejected", "task_id", task.ID, "run_id", run.ID, "error", cause)
}

func (a *App) beginScheduledTaskRun(ctx context.Context, task domain.ScheduledTask, trigger domain.ScheduledTaskTrigger, scheduledFor *time.Time) (domain.ScheduledTaskRun, bool, error) {
	now := time.Now().UTC()
	a.scheduledRunsMu.Lock()
	if _, exists := a.scheduledRuns[task.ID]; exists {
		a.scheduledRunsMu.Unlock()
		finishedAt := now
		run := domain.ScheduledTaskRun{
			ID:             id.New("trn"),
			TaskID:         task.ID,
			OrganizationID: task.OrganizationID,
			ProjectID:      task.ProjectID,
			Kind:           task.Kind,
			Trigger:        trigger,
			ScheduledFor:   scheduledFor,
			StartedAt:      now,
			FinishedAt:     &finishedAt,
			Status:         domain.ScheduledTaskRunStatusSkipped,
			Error:          "previous run still active",
		}
		nextRunAt := nextScheduledTaskRunAtOrNil(task, now)
		if err := a.store.ScheduledTasks().CreateRun(ctx, run); err != nil {
			return domain.ScheduledTaskRun{}, false, err
		}
		if err := a.store.ScheduledTasks().UpdateScheduleState(ctx, task.ID, run.ID, string(run.Status), &run.StartedAt, run.FinishedAt, nextRunAt, now); err != nil {
			return domain.ScheduledTaskRun{}, false, err
		}
		return run, false, nil
	}
	a.scheduledRuns[task.ID] = struct{}{}
	a.scheduledRunsMu.Unlock()

	run := domain.ScheduledTaskRun{
		ID:             id.New("trn"),
		TaskID:         task.ID,
		OrganizationID: task.OrganizationID,
		ProjectID:      task.ProjectID,
		Kind:           task.Kind,
		Trigger:        trigger,
		ScheduledFor:   scheduledFor,
		StartedAt:      now,
		Status:         domain.ScheduledTaskRunStatusRunning,
	}
	if err := a.store.ScheduledTasks().CreateRun(ctx, run); err != nil {
		a.clearScheduledTaskRunning(task.ID)
		return domain.ScheduledTaskRun{}, false, err
	}
	nextRunAt := nextScheduledTaskRunAtOrNil(task, now)
	if err := a.store.ScheduledTasks().UpdateScheduleState(ctx, task.ID, run.ID, string(run.Status), &run.StartedAt, nil, nextRunAt, now); err != nil {
		a.clearScheduledTaskRunning(task.ID)
		return domain.ScheduledTaskRun{}, false, err
	}
	return run, true, nil
}

func (a *App) failScheduledTaskRunStart(ctx context.Context, task domain.ScheduledTask, run domain.ScheduledTaskRun, cause error) {
	a.clearScheduledTaskRunning(task.ID)
	run.Status = domain.ScheduledTaskRunStatusFailed
	run.Error = cause.Error()
	a.finishScheduledTaskRun(ctx, task, run)
}

func (a *App) executeScheduledTaskRun(ctx context.Context, task domain.ScheduledTask, run domain.ScheduledTaskRun) {
	defer a.clearScheduledTaskRunning(task.ID)
	defer func() {
		if recovered := recover(); recovered != nil {
			run.Status = domain.ScheduledTaskRunStatusFailed
			run.Error = fmt.Sprintf("scheduled task panic: %v", recovered)
			a.finishScheduledTaskRun(context.Background(), task, run)
		}
	}()

	switch task.Kind {
	case domain.ScheduledTaskKindAgentPrompt:
		outcome, err := a.executeScheduledAgentPrompt(ctx, task, run)
		run.MessageID = outcome.MessageID
		if err != nil {
			run.Status = domain.ScheduledTaskRunStatusFailed
			run.Error = err.Error()
		} else {
			run.Status = domain.ScheduledTaskRunStatusCompleted
		}
	case domain.ScheduledTaskKindForumPost:
		outcome, err := a.executeScheduledForumPost(ctx, task, run)
		run.MessageID = outcome.MessageID
		run.ThreadID = outcome.ThreadID
		if err != nil {
			run.Status = domain.ScheduledTaskRunStatusFailed
			run.Error = err.Error()
		} else {
			run.Status = domain.ScheduledTaskRunStatusCompleted
		}
	case domain.ScheduledTaskKindShellCommand:
		a.executeScheduledShellCommand(ctx, task, &run)
	default:
		run.Status = domain.ScheduledTaskRunStatusFailed
		run.Error = "unknown task kind"
	}
	a.finishScheduledTaskRun(context.Background(), task, run)
}

func (a *App) finishScheduledTaskRun(ctx context.Context, task domain.ScheduledTask, run domain.ScheduledTaskRun) {
	now := time.Now().UTC()
	run.FinishedAt = &now
	if err := a.store.ScheduledTasks().UpdateRun(ctx, run); err != nil {
		slog.Error("scheduled task run update failed", "task_id", task.ID, "run_id", run.ID, "error", err)
		return
	}
	nextRunAt := nextScheduledTaskRunAtOrNil(task, now)
	if err := a.store.ScheduledTasks().UpdateScheduleState(ctx, task.ID, run.ID, string(run.Status), &run.StartedAt, run.FinishedAt, nextRunAt, now); err != nil {
		slog.Error("scheduled task state update failed", "task_id", task.ID, "run_id", run.ID, "error", err)
	}
}

func (a *App) clearScheduledTaskRunning(taskID string) {
	a.scheduledRunsMu.Lock()
	defer a.scheduledRunsMu.Unlock()
	delete(a.scheduledRuns, taskID)
}

type scheduledPromptOutcome struct {
	MessageID string
	ThreadID  string
}

func (a *App) executeScheduledAgentPrompt(ctx context.Context, task domain.ScheduledTask, run domain.ScheduledTaskRun) (scheduledPromptOutcome, error) {
	return a.dispatchScheduledPrompt(ctx, task, run, task.ConversationType, task.ConversationID)
}

func (a *App) executeScheduledForumPost(ctx context.Context, task domain.ScheduledTask, run domain.ScheduledTaskRun) (scheduledPromptOutcome, error) {
	channel, err := a.store.Channels().ByID(ctx, task.ConversationID)
	if err != nil {
		return scheduledPromptOutcome{}, err
	}
	if channel.Type != domain.ChannelTypeThread {
		return scheduledPromptOutcome{}, invalidInput("forum post tasks require a forum channel")
	}
	if channel.ArchivedAt != nil {
		return scheduledPromptOutcome{}, invalidInput("target forum channel is archived")
	}
	title, err := renderScheduledPostTitle(task, time.Now().UTC())
	if err != nil {
		return scheduledPromptOutcome{}, err
	}
	now := time.Now().UTC()
	thread := domain.Thread{
		ID:             id.New("thr"),
		OrganizationID: channel.OrganizationID,
		ProjectID:      channel.ProjectID,
		ChannelID:      channel.ID,
		Title:          title,
		CreatedBy:      task.CreatedBy,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := a.store.Threads().Create(ctx, thread); err != nil {
		return scheduledPromptOutcome{}, err
	}
	if err := a.applyScheduledForumPostMembers(ctx, task, thread, now); err != nil {
		return scheduledPromptOutcome{}, err
	}
	outcome, err := a.dispatchScheduledPrompt(ctx, task, run, domain.ConversationThread, thread.ID)
	outcome.ThreadID = thread.ID
	return outcome, err
}

// applyScheduledForumPostMembers scopes a scheduled post to its target agent,
// matching what an @mention does for a hand-written post. Tasks that target no
// specific agent leave the post open to every channel agent.
func (a *App) applyScheduledForumPostMembers(ctx context.Context, task domain.ScheduledTask, thread domain.Thread, now time.Time) error {
	channelAgents, err := a.ChannelAgents(ctx, thread.ChannelID)
	if err != nil {
		return err
	}
	var targets []ConversationAgentContext
	if task.AgentID != "" {
		for _, agent := range channelAgents {
			if agent.Agent.ID == task.AgentID {
				targets = []ConversationAgentContext{agent}
				break
			}
		}
	} else {
		targets = mentionedAgentsForBody(channelAgents, task.Prompt)
	}
	if len(targets) == 0 {
		return nil
	}
	return a.store.ThreadAgents().ReplaceForThread(ctx, thread.ID, threadAgentRows(thread.ID, targets, now))
}

func (a *App) dispatchScheduledPrompt(
	ctx context.Context,
	task domain.ScheduledTask,
	run domain.ScheduledTaskRun,
	conversationType domain.ConversationType,
	conversationID string,
) (scheduledPromptOutcome, error) {
	scope, err := a.conversationScope(ctx, conversationType, conversationID)
	if err != nil {
		return scheduledPromptOutcome{}, err
	}
	agents, err := a.conversationAgents(ctx, scope)
	if err != nil {
		return scheduledPromptOutcome{}, err
	}
	if len(agents) == 0 {
		return scheduledPromptOutcome{}, invalidInput("target conversation has no enabled agents")
	}
	targets := agents
	if task.AgentID != "" {
		targets = nil
		for _, target := range agents {
			if target.Agent.ID == task.AgentID {
				targets = []ConversationAgentContext{target}
				break
			}
		}
		if len(targets) == 0 {
			return scheduledPromptOutcome{}, invalidInput("agent is not bound to the target conversation")
		}
	} else if mentioned := mentionedAgentsForBody(agents, task.Prompt); len(mentioned) > 0 {
		targets = mentioned
	}
	if task.FreshContext {
		boundary := time.Now().UTC()
		for _, target := range targets {
			if err := a.store.Sessions().ResetAgentSessionContext(ctx, target.Agent.ID, conversationType, conversationID, boundary); err != nil {
				return scheduledPromptOutcome{}, err
			}
		}
	}
	metadata := map[string]any{
		"scheduled":           true,
		"scheduled_task_id":   task.ID,
		"scheduled_task_name": task.Name,
		"scheduled_run_id":    run.ID,
		"scheduled_trigger":   string(run.Trigger),
	}
	if task.FreshContext {
		metadata["scheduled_fresh_context"] = true
	}
	if !task.Notify {
		// Marks the run so replies it triggers skip webhook and browser notifications.
		metadata[suppressNotificationsMetadataKey] = true
	}
	message, err := a.createConversationMessage(ctx, SendMessageRequest{
		UserID:           task.CreatedBy,
		OrganizationID:   task.OrganizationID,
		ConversationType: conversationType,
		ConversationID:   conversationID,
		Body:             task.Prompt,
	}, domain.SenderSystem, "scheduled", task.Prompt, metadata)
	if err != nil {
		return scheduledPromptOutcome{}, err
	}
	outcome := scheduledPromptOutcome{MessageID: message.ID}
	errCh := make(chan error, len(targets))
	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(target ConversationAgentContext) {
			defer wg.Done()
			if err := a.runScheduledAgentTarget(ctx, message, target); err != nil {
				errCh <- err
			}
		}(target)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return outcome, err
		}
	}
	return outcome, nil
}

func renderScheduledPostTitle(task domain.ScheduledTask, now time.Time) (string, error) {
	location, err := time.LoadLocation(strings.TrimSpace(task.Timezone))
	if err != nil {
		location = time.UTC
	}
	local := now.In(location)
	template := strings.TrimSpace(task.PostTitle)
	if template == "" {
		template = defaultScheduledPostTitleTemplate
	}
	title := strings.NewReplacer(
		"${task}", task.Name,
		"${date}", local.Format("2006-01-02"),
		"${time}", local.Format("15:04"),
		"${datetime}", local.Format("2006-01-02 15:04"),
	).Replace(template)
	title = strings.TrimSpace(title)
	if title == "" {
		return "", invalidInput("post title renders to an empty value")
	}
	if len([]rune(title)) > maxScheduledPostTitleLength {
		title = strings.TrimSpace(string([]rune(title)[:maxScheduledPostTitleLength]))
	}
	return title, nil
}

func (a *App) runScheduledAgentTarget(ctx context.Context, message domain.Message, target ConversationAgentContext) error {
	result := make(chan agentRunResult, 1)
	a.runAgentForMessageWithTarget(ctx, message, target, id.New("run"), agentRunOptions{Result: result})
	runResult := <-result
	return runResult.Err
}

func (a *App) executeScheduledShellCommand(ctx context.Context, task domain.ScheduledTask, run *domain.ScheduledTaskRun) {
	if !a.opts.ScheduledShellEnabled {
		run.Status = domain.ScheduledTaskRunStatusFailed
		run.Error = "scheduled shell tasks are disabled"
		return
	}
	workspace, err := a.store.Workspaces().ByID(ctx, task.WorkspaceID)
	if err != nil {
		run.Status = domain.ScheduledTaskRunStatusFailed
		run.Error = err.Error()
		return
	}
	if err := os.MkdirAll(workspace.Path, 0o755); err != nil {
		run.Status = domain.ScheduledTaskRunStatusFailed
		run.Error = err.Error()
		return
	}
	timeout := time.Duration(task.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultScheduledTaskTimeoutSeconds * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	name, args := scheduledShellCommand(task.Command)
	cmd := exec.CommandContext(runCtx, name, args...)
	cmd.Dir = workspace.Path
	stdout := &limitedOutputBuffer{limit: scheduledTaskOutputLimit}
	stderr := &limitedOutputBuffer{limit: scheduledTaskOutputLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	run.Stdout = stdout.String()
	run.Stderr = stderr.String()
	run.OutputTruncated = stdout.Truncated() || stderr.Truncated()
	if err == nil {
		exitCode := 0
		run.ExitCode = &exitCode
		run.Status = domain.ScheduledTaskRunStatusCompleted
		return
	}
	run.Status = domain.ScheduledTaskRunStatusFailed
	if runCtx.Err() != nil {
		run.Error = "command timed out"
		return
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode := exitErr.ExitCode()
		run.ExitCode = &exitCode
		run.Error = fmt.Sprintf("command exited with status %d", exitCode)
		return
	}
	run.Error = err.Error()
}

func scheduledShellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", command}
	}
	return "/bin/sh", []string{"-lc", command}
}

type scheduledTaskScheduler struct {
	app     *App
	cron    *cron.Cron
	entries map[string]cron.EntryID
	mu      sync.Mutex
}

func newScheduledTaskScheduler(a *App) *scheduledTaskScheduler {
	parser := scheduledTaskCronParser()
	return &scheduledTaskScheduler{
		app:     a,
		cron:    cron.New(cron.WithParser(parser), cron.WithLocation(time.UTC), cron.WithChain(cron.Recover(cronSlogLogger{}))),
		entries: make(map[string]cron.EntryID),
	}
}

func (s *scheduledTaskScheduler) start() {
	s.cron.Start()
}

func (s *scheduledTaskScheduler) stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

func (s *scheduledTaskScheduler) upsert(ctx context.Context, task domain.ScheduledTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.entries[task.ID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, task.ID)
	}
	var nextRunAt *time.Time
	if task.Enabled {
		scheduleSpec := scheduledTaskScheduleSpec(task)
		entryID, err := s.cron.AddFunc(scheduleSpec, func() {
			scheduledFor := time.Now().UTC()
			s.app.startScheduledTaskFromCron(task.ID, scheduledFor)
		})
		if err != nil {
			return err
		}
		s.entries[task.ID] = entryID
		next := nextScheduledTaskRunAtOrNil(task, time.Now().UTC())
		nextRunAt = next
	}
	return s.app.store.ScheduledTasks().UpdateScheduleState(ctx, task.ID, task.LastRunID, task.LastRunStatus, task.LastRunAt, task.LastFinishedAt, nextRunAt, time.Now().UTC())
}

func (s *scheduledTaskScheduler) remove(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.entries[taskID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, taskID)
	}
}

func scheduledTaskCronParser() cron.Parser {
	return cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
}

func scheduledTaskScheduleSpec(task domain.ScheduledTask) string {
	timezone := strings.TrimSpace(task.Timezone)
	if timezone == "" {
		timezone = defaultScheduledTaskTimezone
	}
	return "CRON_TZ=" + timezone + " " + strings.TrimSpace(task.Schedule)
}

func nextScheduledTaskRunAt(task domain.ScheduledTask, after time.Time) (time.Time, error) {
	if _, err := time.LoadLocation(task.Timezone); err != nil {
		return time.Time{}, err
	}
	schedule, err := scheduledTaskCronParser().Parse(scheduledTaskScheduleSpec(task))
	if err != nil {
		return time.Time{}, err
	}
	next := schedule.Next(after)
	if next.IsZero() {
		return time.Time{}, errors.New("schedule has no next run")
	}
	return next.UTC(), nil
}

func nextScheduledTaskRunAtOrNil(task domain.ScheduledTask, after time.Time) *time.Time {
	if !task.Enabled {
		return nil
	}
	next, err := nextScheduledTaskRunAt(task, after)
	if err != nil {
		return nil
	}
	return &next
}

type cronSlogLogger struct{}

func (cronSlogLogger) Info(msg string, keysAndValues ...any) {
	slog.Info(msg, keysAndValues...)
}

func (cronSlogLogger) Error(err error, msg string, keysAndValues ...any) {
	args := append(keysAndValues, "error", err)
	slog.Error(msg, args...)
}

type limitedOutputBuffer struct {
	limit     int
	buf       bytes.Buffer
	truncated bool
}

func (b *limitedOutputBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		b.truncated = true
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *limitedOutputBuffer) String() string {
	return b.buf.String()
}

func (b *limitedOutputBuffer) Truncated() bool {
	return b.truncated
}
