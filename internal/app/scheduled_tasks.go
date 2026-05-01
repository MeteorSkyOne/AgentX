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
	TimeoutSeconds   int
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
	go a.executeScheduledTaskRun(context.Background(), task, run)
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
	case domain.ScheduledTaskKindShellCommand:
		return a.normalizeShellCommandTask(task)
	default:
		return domain.ScheduledTask{}, invalidInput("unknown task kind")
	}
}

func (a *App) normalizeAgentPromptTask(ctx context.Context, project domain.Project, task domain.ScheduledTask) (domain.ScheduledTask, error) {
	task.Prompt = strings.TrimSpace(task.Prompt)
	task.Command = ""
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
	if scope.organizationID != project.OrganizationID {
		return domain.ScheduledTask{}, invalidInput("target conversation must belong to the task organization")
	}
	if scope.project.ID != "" && scope.project.ID != project.ID {
		return domain.ScheduledTask{}, invalidInput("target conversation must belong to the task project")
	}
	agents, err := a.conversationAgents(ctx, scope)
	if err != nil {
		return domain.ScheduledTask{}, err
	}
	task.AgentID = strings.TrimSpace(task.AgentID)
	if task.AgentID != "" {
		found := false
		for _, item := range agents {
			if item.Agent.ID == task.AgentID {
				found = true
				break
			}
		}
		if !found {
			return domain.ScheduledTask{}, invalidInput("agent is not bound to the target conversation")
		}
	}
	return task, nil
}

func (a *App) normalizeShellCommandTask(task domain.ScheduledTask) (domain.ScheduledTask, error) {
	if !a.opts.ScheduledShellEnabled {
		return domain.ScheduledTask{}, invalidInput("scheduled shell tasks are disabled")
	}
	task.Command = strings.TrimSpace(task.Command)
	task.Prompt = ""
	task.ConversationType = ""
	task.ConversationID = ""
	task.AgentID = ""
	if task.Command == "" {
		return domain.ScheduledTask{}, invalidInput("command is required")
	}
	return task, nil
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
		messageID, err := a.executeScheduledAgentPrompt(ctx, task, run)
		run.MessageID = messageID
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

func (a *App) executeScheduledAgentPrompt(ctx context.Context, task domain.ScheduledTask, run domain.ScheduledTaskRun) (string, error) {
	scope, err := a.conversationScope(ctx, task.ConversationType, task.ConversationID)
	if err != nil {
		return "", err
	}
	agents, err := a.conversationAgents(ctx, scope)
	if err != nil {
		return "", err
	}
	if len(agents) == 0 {
		return "", invalidInput("target conversation has no enabled agents")
	}
	metadata := map[string]any{
		"scheduled":           true,
		"scheduled_task_id":   task.ID,
		"scheduled_task_name": task.Name,
		"scheduled_run_id":    run.ID,
		"scheduled_trigger":   string(run.Trigger),
	}
	message, err := a.createConversationMessage(ctx, SendMessageRequest{
		UserID:           task.CreatedBy,
		OrganizationID:   task.OrganizationID,
		ConversationType: task.ConversationType,
		ConversationID:   task.ConversationID,
		Body:             task.Prompt,
	}, domain.SenderSystem, "scheduled", task.Prompt, metadata)
	if err != nil {
		return "", err
	}
	if task.AgentID != "" {
		for _, target := range agents {
			if target.Agent.ID == task.AgentID {
				return message.ID, a.runScheduledAgentTarget(ctx, message, target)
			}
		}
		return message.ID, invalidInput("agent is not bound to the target conversation")
	}
	if targets := mentionedAgentsForBody(agents, message.Body); len(targets) > 0 {
		a.runAgentTeamForMessage(ctx, message, scope, agents, targets)
		return message.ID, nil
	}
	errCh := make(chan error, len(agents))
	var wg sync.WaitGroup
	for _, target := range agents {
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
			return message.ID, err
		}
	}
	return message.ID, nil
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
			s.app.runScheduledTask(context.Background(), task.ID, domain.ScheduledTaskTriggerScheduled, &scheduledFor)
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
