package codexpersist

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

type inputAnswer struct {
	questionID string
	answer     string
}

var errNoActiveTurn = errors.New("no active turn")

type sessionRPC interface {
	Call(ctx context.Context, method string, params any) (map[string]any, error)
	Notifications() <-chan jsonRPCMessage
	RespondToRequest(id any, result any) error
}

type persistentSession struct {
	process  *procpool.ManagedProcess
	rpc      sessionRPC
	key      string
	rt       *Runtime
	req      runtime.StartSessionRequest
	events   chan runtime.Event
	threadID string
	model    string

	mu                 sync.Mutex
	eventMu            sync.Mutex
	sessionID          string
	alive              bool
	started            bool
	turnHeld           bool
	activeTurnID       string
	interruptRequested bool
	interruptSent      bool
	interruptCh        chan struct{}
	interruptOnce      sync.Once
	done               chan struct{}
	closeOnce          sync.Once
	pendingInput       chan inputAnswer
}

func newPersistentSession(proc *procpool.ManagedProcess, rpc *rpcClient, key string, rt *Runtime, req runtime.StartSessionRequest) *persistentSession {
	fallbackID := "codex:" + key
	threadID := usablePreviousSessionID(req.PreviousSessionID)
	sessionID := fallbackID
	if threadID != "" {
		sessionID = threadID
	}
	return &persistentSession{
		process:      proc,
		rpc:          rpc,
		key:          key,
		rt:           rt,
		req:          req,
		events:       make(chan runtime.Event, 64),
		threadID:     threadID,
		sessionID:    sessionID,
		alive:        true,
		interruptCh:  make(chan struct{}),
		done:         make(chan struct{}),
		pendingInput: make(chan inputAnswer, 1),
	}
}

func (s *persistentSession) Send(ctx context.Context, input runtime.Input) error {
	s.mu.Lock()
	if !s.alive {
		s.mu.Unlock()
		return procpool.ErrProcessDead
	}
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = true
	s.mu.Unlock()

	if err := s.process.AcquireTurn(ctx); err != nil {
		s.emitFailed(err.Error())
		return nil
	}
	s.mu.Lock()
	s.turnHeld = true
	s.mu.Unlock()

	go s.runTurn(ctx, input)
	return nil
}

func (s *persistentSession) Events() <-chan runtime.Event {
	return s.events
}

func (s *persistentSession) CurrentSessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionID
}

func (s *persistentSession) Alive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alive
}

func (s *persistentSession) RespondToInputRequest(questionID string, answer string) error {
	select {
	case s.pendingInput <- inputAnswer{questionID: questionID, answer: answer}:
		return nil
	default:
		return errors.New("no pending input request")
	}
}

func (s *persistentSession) Close(ctx context.Context) error {
	s.mu.Lock()
	s.alive = false
	turnHeld := s.turnHeld
	s.turnHeld = false
	s.mu.Unlock()

	if turnHeld {
		s.process.ReleaseTurn()
	}
	s.closeEventStream()
	return nil
}

func (s *persistentSession) Stop(ctx context.Context) error {
	s.InitiateStop()

	done := make(chan struct{})
	go func() {
		s.process.Kill()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *persistentSession) Interrupt(ctx context.Context) error {
	threadID, turnID, shouldSend, err := s.requestInterrupt()
	if err != nil || !shouldSend {
		return err
	}
	return s.sendTurnInterrupt(ctx, threadID, turnID)
}

func (s *persistentSession) Steer(ctx context.Context, input runtime.Input) error {
	threadID, turnID, err := s.activeTurn()
	if err != nil {
		return err
	}
	_, err = s.rpc.Call(ctx, "turn/steer", map[string]any{
		"threadId":       threadID,
		"expectedTurnId": turnID,
		"input":          buildUserInput(input),
	})
	return err
}

func (s *persistentSession) CanSteer() bool {
	_, _, err := s.activeTurn()
	return err == nil
}

func (s *persistentSession) activeTurn() (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.alive {
		return "", "", procpool.ErrProcessDead
	}
	if s.threadID == "" || s.activeTurnID == "" {
		return "", "", errNoActiveTurn
	}
	return s.threadID, s.activeTurnID, nil
}

func (s *persistentSession) requestInterrupt() (string, string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.alive {
		return "", "", false, procpool.ErrProcessDead
	}
	s.interruptRequested = true
	if s.interruptCh != nil {
		s.interruptOnce.Do(func() {
			close(s.interruptCh)
		})
	}
	if s.threadID == "" || s.activeTurnID == "" || s.interruptSent {
		return "", "", false, nil
	}
	s.interruptSent = true
	return s.threadID, s.activeTurnID, true, nil
}

func (s *persistentSession) interruptActiveTurnIfRequested(ctx context.Context) error {
	s.mu.Lock()
	if !s.interruptRequested || s.interruptSent || s.threadID == "" || s.activeTurnID == "" {
		s.mu.Unlock()
		return nil
	}
	threadID := s.threadID
	turnID := s.activeTurnID
	s.interruptSent = true
	s.mu.Unlock()
	return s.sendTurnInterrupt(ctx, threadID, turnID)
}

func (s *persistentSession) sendTurnInterrupt(ctx context.Context, threadID string, turnID string) error {
	_, err := s.rpc.Call(ctx, "turn/interrupt", map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
	})
	return err
}

func (s *persistentSession) InitiateStop() {
	s.mu.Lock()
	s.alive = false
	turnHeld := s.turnHeld
	s.turnHeld = false
	s.mu.Unlock()

	if turnHeld {
		s.process.ReleaseTurn()
	}
	s.rt.pool.Detach(s.process)
	s.closeEventStream()
}

func (s *persistentSession) runTurn(ctx context.Context, input runtime.Input) {
	defer func() {
		s.releaseTurn()
		s.clearActiveTurn()
		s.mu.Lock()
		s.alive = false
		s.mu.Unlock()
		s.closeEventStream()
	}()

	if s.threadID == "" {
		started, err := s.startThread(ctx)
		if err != nil {
			s.emit(runtime.Event{Type: runtime.EventFailed, Error: err.Error()})
			return
		}
		s.threadID = started.threadID
		s.model = started.model
		s.mu.Lock()
		s.sessionID = started.threadID
		s.mu.Unlock()
	}

	if objective, isClear, ok := parseGoalPrompt(input.Prompt); ok {
		if isClear {
			s.handleGoalClear(ctx)
		} else {
			s.handleGoalSet(ctx, input, objective)
		}
		return
	}

	s.startTurnAndProcess(ctx, input)
}

func (s *persistentSession) startTurnAndProcess(ctx context.Context, input runtime.Input) {
	userInput := buildUserInput(input)
	turnParams := map[string]any{
		"threadId": s.threadID,
		"input":    userInput,
	}
	if model := strings.TrimSpace(s.req.Model); model != "" {
		turnParams["model"] = model
	}
	if effort := strings.TrimSpace(s.req.Effort); effort != "" {
		turnParams["effort"] = effort
	}
	s.addTurnOverrides(turnParams)

	result, err := s.rpc.Call(ctx, "turn/start", turnParams)
	if err != nil {
		s.emit(rpcFailureEvent("turn/start failed", err))
		return
	}
	turnID := turnIDFromResult(result)
	s.setActiveTurnID(turnID)
	if err := s.interruptActiveTurnIfRequested(ctx); err != nil {
		s.emit(rpcFailureEvent("turn/interrupt failed", err))
		return
	}

	s.processNotifications(ctx)
}

func (s *persistentSession) handleGoalSet(ctx context.Context, input runtime.Input, objective string) {
	_, err := s.rpc.Call(ctx, "thread/goal/set", map[string]any{
		"threadId":  s.threadID,
		"objective": objective,
	})
	if err != nil {
		s.emit(rpcFailureEvent("thread/goal/set failed", err))
		return
	}
	goalInput := runtime.Input{
		Prompt:      objective,
		Context:     input.Context,
		Attachments: input.Attachments,
	}
	s.startTurnAndProcess(ctx, goalInput)
}

func (s *persistentSession) handleGoalClear(ctx context.Context) {
	_, err := s.rpc.Call(ctx, "thread/goal/clear", map[string]any{
		"threadId": s.threadID,
	})
	if err != nil {
		s.emit(rpcFailureEvent("thread/goal/clear failed", err))
		return
	}
	s.emit(runtime.Event{Type: runtime.EventCompleted, Text: "Goal cleared."})
}

func parseGoalPrompt(prompt string) (objective string, isClear bool, ok bool) {
	prompt = strings.TrimSpace(prompt)
	lower := strings.ToLower(prompt)
	if !strings.HasPrefix(lower, "/goal ") && lower != "/goal" {
		return "", false, false
	}
	args := strings.TrimSpace(prompt[len("/goal"):])
	if strings.EqualFold(args, "clear") {
		return "", true, true
	}
	if args == "" {
		return "", false, false
	}
	return args, false, true
}

type threadStartResult struct {
	threadID string
	model    string
}

func (s *persistentSession) startThread(ctx context.Context) (threadStartResult, error) {
	workspace := strings.TrimSpace(s.req.Workspace)
	if workspace == "" {
		workspace = "."
	}

	params := map[string]any{
		"cwd": workspace,
	}
	if model := strings.TrimSpace(s.req.Model); model != "" {
		params["model"] = model
	}
	s.addThreadOverrides(params)

	result, err := s.rpc.Call(ctx, "thread/start", params)
	if err != nil {
		return threadStartResult{}, fmt.Errorf("thread/start: %w", err)
	}

	thread, _ := result["thread"].(map[string]any)
	if thread == nil {
		return threadStartResult{}, fmt.Errorf("thread/start: missing thread in response")
	}
	threadID, _ := thread["id"].(string)
	if threadID == "" {
		return threadStartResult{}, fmt.Errorf("thread/start: missing thread id")
	}
	return threadStartResult{threadID: threadID, model: stringVal(result, "model")}, nil
}

func (s *persistentSession) addTurnOverrides(turnParams map[string]any) {
	mode := "default"
	if strings.EqualFold(strings.TrimSpace(s.req.PermissionMode), "plan") {
		mode = "plan"
	}
	if s.req.YoloMode {
		turnParams["approvalPolicy"] = "never"
	}
	turnParams["sandboxPolicy"] = codexSandboxPolicy(mode, s.req.YoloMode)

	effort := strings.TrimSpace(s.req.Effort)
	if mode == "plan" && effort == "" {
		effort = "medium"
	}

	settings := map[string]any{
		"model":                  s.turnModel(),
		"developer_instructions": nil,
		"reasoning_effort":       nil,
	}
	if effort != "" {
		settings["reasoning_effort"] = effort
	}

	turnParams["collaborationMode"] = map[string]any{
		"mode":     mode,
		"settings": settings,
	}
}

func (s *persistentSession) addThreadOverrides(params map[string]any) {
	if !s.req.YoloMode {
		return
	}
	params["approvalPolicy"] = "never"
	params["sandbox"] = "danger-full-access"
}

func codexSandboxPolicy(mode string, yolo bool) map[string]any {
	if mode == "plan" {
		return map[string]any{"type": "readOnly"}
	}
	if yolo {
		return map[string]any{"type": "dangerFullAccess"}
	}
	return map[string]any{"type": "workspaceWrite"}
}

func (s *persistentSession) turnModel() string {
	if model := strings.TrimSpace(s.req.Model); model != "" {
		return model
	}
	return strings.TrimSpace(s.model)
}

func (s *persistentSession) setActiveTurnID(turnID string) {
	if turnID == "" {
		return
	}
	s.mu.Lock()
	s.activeTurnID = turnID
	s.mu.Unlock()
}

func (s *persistentSession) clearActiveTurn() {
	s.mu.Lock()
	s.activeTurnID = ""
	s.mu.Unlock()
}
