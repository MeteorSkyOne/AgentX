package agy

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

const (
	cancelGraceTimeout = 10 * time.Second
	closeSessionWait   = 3 * time.Second
)

type staleSessionError struct {
	cause error
}

func (e staleSessionError) Error() string {
	return e.cause.Error()
}

func (e staleSessionError) Unwrap() error {
	return e.cause
}

type inputAnswer struct {
	questionID string
	answer     string
}

type persistentSession struct {
	process *procpool.ManagedProcess
	rpc     *rpcClient
	key     string
	rt      *Runtime
	req     runtime.StartSessionRequest
	events  chan runtime.Event

	mu                 sync.Mutex
	eventMu            sync.Mutex
	sessionID          string
	needsNewSession    bool
	alive              bool
	started            bool
	turnHeld           bool
	running            bool
	interruptRequested bool
	interruptCh        chan struct{}
	interruptOnce      sync.Once
	done               chan struct{}
	closeOnce          sync.Once
	pendingInput       chan inputAnswer
	questionSeq        atomic.Int64
}

func newPersistentSession(proc *procpool.ManagedProcess, rpc *rpcClient, key string, rt *Runtime, req runtime.StartSessionRequest) *persistentSession {
	sid := usablePreviousSessionID(req.PreviousSessionID)
	if sid == "" {
		sid = "agy:" + key
	}
	return &persistentSession{
		process:      proc,
		rpc:          rpc,
		key:          key,
		rt:           rt,
		req:          req,
		events:       make(chan runtime.Event, 256),
		sessionID:    sid,
		alive:        true,
		interruptCh:  make(chan struct{}),
		done:         make(chan struct{}),
		pendingInput: make(chan inputAnswer, 1),
	}
}

func (s *persistentSession) initializeProcess(ctx context.Context) error {
	initCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()

	if !s.rpc.isInitialized() {
		_, err := s.rpc.CallMap(initCtx, "initialize", record{
			"protocolVersion":    1,
			"clientCapabilities": record{},
			"clientInfo": record{
				"name":    "agentx",
				"title":   "AgentX",
				"version": "0.1.0",
			},
		})
		if err != nil {
			if stderr := strings.TrimSpace(s.process.Stderr()); stderr != "" {
				return fmt.Errorf("agy acp failed to initialize: %s", truncateDetail(stderr))
			}
			return fmt.Errorf("agy acp failed to initialize: %w", err)
		}
		s.rpc.setInitialized()
	}
	return nil
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
		s.emitFailed(err.Error(), false)
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
		return fmt.Errorf("no pending input request")
	}
}

func (s *persistentSession) Close(_ context.Context) error {
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

func (s *persistentSession) Interrupt(_ context.Context) error {
	s.mu.Lock()
	if !s.alive {
		s.mu.Unlock()
		return procpool.ErrProcessDead
	}
	s.interruptRequested = true
	running := s.running
	sid := usablePreviousSessionID(s.sessionID)
	s.mu.Unlock()

	s.interruptOnce.Do(func() { close(s.interruptCh) })
	if !running || sid == "" {
		return nil
	}
	return s.rpc.Notify("session/cancel", record{"sessionId": sid})
}

func (s *persistentSession) ContextUsage(ctx context.Context) (*runtime.ContextUsage, error) {
	s.mu.Lock()
	alive := s.alive
	s.mu.Unlock()
	if !alive {
		return nil, procpool.ErrProcessDead
	}

	if err := s.process.AcquireTurn(ctx); err != nil {
		return nil, err
	}
	defer s.process.ReleaseTurn()

	if usage := s.rpc.contextUsage(); usage != nil {
		activeID, _, _, _ := s.rpc.sessionState()
		prevID := usablePreviousSessionID(s.req.PreviousSessionID)
		if prevID == "" || activeID == prevID {
			return usage, nil
		}
	}

	if prevID := usablePreviousSessionID(s.req.PreviousSessionID); prevID != "" {
		if err := s.ensureSession(ctx); err != nil {
			var stale staleSessionError
			if errors.As(err, &stale) {
				return nil, nil
			}
			return nil, err
		}
		// Wait briefly for `agy acp` to deliver its initial `usage_update` notification.
		deadline := time.NewTimer(500 * time.Millisecond)
		defer deadline.Stop()
		for s.rpc.contextUsage() == nil {
			select {
			case _, ok := <-s.rpc.Messages():
				if !ok {
					return s.rpc.contextUsage(), nil
				}
			case <-ctx.Done():
				return s.rpc.contextUsage(), ctx.Err()
			case <-deadline.C:
				s.rpc.drainAndCacheUpdates()
				return s.rpc.contextUsage(), nil
			}
		}
		s.rpc.drainAndCacheUpdates()
	}
	return s.rpc.contextUsage(), nil
}

func (s *persistentSession) runTurn(ctx context.Context, input runtime.Input) {
	defer func() {
		s.setRunning(false)
		s.releaseTurn()
		s.mu.Lock()
		s.alive = false
		s.mu.Unlock()
		s.closeEventStream()
	}()

	started := time.Now()
	if err := s.ensureSession(ctx); err != nil {
		var stale staleSessionError
		if errors.As(err, &stale) {
			s.emit(runtime.Event{
				Type:         runtime.EventFailed,
				Error:        stale.Error(),
				StaleSession: true,
			})
			return
		}
		s.emit(runtime.Event{Type: runtime.EventFailed, Error: s.formatTurnError(err)})
		return
	}

	if err := s.reconcileConfig(ctx); err != nil {
		s.emit(runtime.Event{Type: runtime.EventFailed, Error: s.formatTurnError(err)})
		return
	}

	// Drain any config/info notifications from session setup before starting the prompt.
	s.rpc.drainAndCacheUpdates()

	sid := s.CurrentSessionID()
	state := newTurnState(input.Prompt)
	state.contextUsage = s.rpc.contextUsage()
	for _, initEvt := range state.initialEvents() {
		s.emit(initEvt)
	}

	blocks := buildPromptBlocks(input)
	promptCtx, cancelPrompt := context.WithCancel(ctx)
	defer cancelPrompt()

	type promptOutcome struct {
		result record
		err    error
	}
	promptDone := make(chan promptOutcome, 1)

	s.setRunning(true)
	if s.wasInterrupted() {
		_ = s.rpc.Notify("session/cancel", record{"sessionId": sid})
	}

	go func() {
		res, err := s.rpc.CallMap(promptCtx, "session/prompt", record{
			"sessionId": sid,
			"prompt":    blocks,
		})
		promptDone <- promptOutcome{result: res, err: err}
	}()

	var cancelTimer *time.Timer
	var cancelTimeoutCh <-chan time.Time
	defer func() {
		if cancelTimer != nil {
			cancelTimer.Stop()
		}
	}()

	interruptHandled := false
	for {
		var intrCh <-chan struct{}
		if !interruptHandled {
			intrCh = s.interruptCh
		}

		select {
		case msg, ok := <-s.rpc.Messages():
			if !ok {
				// Process exited; wait briefly for promptDone to carry the error.
				out := <-promptDone
				s.finishTurn(state, out.result, out.err, started)
				return
			}
			s.handleTurnMessage(promptCtx, sid, state, msg)

		case <-intrCh:
			interruptHandled = true
			_ = s.rpc.Notify("session/cancel", record{"sessionId": sid})
			cancelTimer = time.NewTimer(cancelGraceTimeout)
			cancelTimeoutCh = cancelTimer.C

		case <-cancelTimeoutCh:
			cancelPrompt()
			cancelTimeoutCh = nil

		case out := <-promptDone:
			s.drainTurnMessages(promptCtx, sid, state)
			s.finishTurn(state, out.result, out.err, started)
			return
		}
	}
}

func (s *persistentSession) drainTurnMessages(ctx context.Context, sid string, state *turnState) {
	// Yield briefly so pumpLoop forwards any notification read immediately before
	// the `session/prompt` response on stdout.
	s.rpc.queueMu.Lock()
	hasQueued := len(s.rpc.queue) > 0
	s.rpc.queueMu.Unlock()
	if hasQueued {
		time.Sleep(5 * time.Millisecond)
	}
	for {
		select {
		case msg, ok := <-s.rpc.Messages():
			if !ok {
				return
			}
			s.handleTurnMessage(ctx, sid, state, msg)
		default:
			s.rpc.queueMu.Lock()
			queued := s.rpc.queue
			s.rpc.queue = nil
			s.rpc.queueMu.Unlock()
			if len(queued) == 0 {
				return
			}
			for _, msg := range queued {
				s.handleTurnMessage(ctx, sid, state, msg)
			}
			return
		}
	}
}

func (s *persistentSession) handleTurnMessage(ctx context.Context, sid string, state *turnState, msg rpcMessage) {
	switch msg.Method {
	case "session/update":
		msgSID := stringVal(msg.Params, "sessionId")
		if msgSID != "" && sid != "" && msgSID != sid {
			return
		}
		update := mapVal(msg.Params, "update")
		for _, evt := range state.handleUpdate(update) {
			s.emit(evt)
		}

	case "session/request_permission":
		if !msg.IDPresent {
			return
		}
		s.handlePermissionRequest(ctx, msg.ID, msg.Params)

	default:
		if msg.IDPresent {
			_ = s.rpc.RespondError(msg.ID, -32601, "Method not found: "+msg.Method)
		}
	}
}

func (s *persistentSession) handlePermissionRequest(ctx context.Context, reqID any, params record) {
	question, toolCallID, choices := permissionQuestionFromParams(params)
	if s.rt.effectiveSkipPermissions(s.req) {
		if allowID := findChoiceByKind(choices, true); allowID != "" {
			_ = s.rpc.Respond(reqID, record{
				"outcome": record{
					"outcome":  "selected",
					"optionId": allowID,
				},
			})
			return
		}
	}

	// Clear any stalebuffered answer before emitting a fresh question.
	for {
		select {
		case <-s.pendingInput:
		default:
			goto ready
		}
	}
ready:
	qid := "agy-perm-" + strconv.FormatInt(s.questionSeq.Add(1), 10)
	opts := make([]runtime.InputRequestOption, 0, len(choices))
	for _, c := range choices {
		opts = append(opts, runtime.InputRequestOption{Label: c.Name})
	}
	s.emit(runtime.Event{
		Type: runtime.EventInputRequest,
		InputRequest: &runtime.InputRequest{
			QuestionID: qid,
			Question:   question,
			ToolCallID: toolCallID,
			RequestID:  reqID,
			Options:    opts,
		},
	})

	select {
	case ans := <-s.pendingInput:
		chosen := selectPermissionOption(choices, ans.answer)
		if chosen == "" {
			_ = s.rpc.Respond(reqID, record{
				"outcome": record{"outcome": "cancelled"},
			})
			return
		}
		_ = s.rpc.Respond(reqID, record{
			"outcome": record{
				"outcome":  "selected",
				"optionId": chosen,
			},
		})
	case <-s.interruptCh:
		_ = s.rpc.Respond(reqID, record{
			"outcome": record{"outcome": "cancelled"},
		})
	case <-ctx.Done():
		_ = s.rpc.Respond(reqID, record{
			"outcome": record{"outcome": "cancelled"},
		})
	case <-s.process.Done():
	}
}

func (s *persistentSession) finishTurn(state *turnState, result record, err error, started time.Time) {
	for _, evt := range state.flushRemaining() {
		s.emit(evt)
	}

	stopReason := strings.ToLower(strings.TrimSpace(stringVal(result, "stopReason")))
	if stopReason == "cancelled" || stopReason == "canceled" || s.wasInterrupted() || errors.Is(err, context.Canceled) {
		_ = state.finishGoal("cancelled", nil)
		s.emit(runtime.Event{Type: runtime.EventCanceled})
		return
	}
	if err != nil {
		s.emit(runtime.Event{
			Type:  runtime.EventFailed,
			Error: s.formatTurnError(err),
		})
		return
	}

	_, modelID, _, _ := s.rpc.sessionState()
	if modelID == "" {
		modelID = strings.TrimSpace(s.req.Model)
	}
	ctxUsage := state.contextUsage
	if ctxUsage == nil {
		ctxUsage = s.rpc.contextUsage()
	}
	usage := buildUsage(result, modelID, time.Since(started).Milliseconds(), ctxUsage)
	var totalTokens *int64
	if usage != nil {
		totalTokens = usage.TotalTokens
	}
	finalGoal := state.finishGoal(stopReason, totalTokens)
	if finalGoal == nil {
		finalGoal = state.goal
	}

	s.emit(runtime.Event{
		Type:  runtime.EventCompleted,
		Text:  state.text.String(),
		Usage: usage,
		Goal:  finalGoal,
		Plan:  state.plan,
	})
}

func (s *persistentSession) ensureSession(ctx context.Context) error {
	if err := s.initializeProcess(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	needsNew := s.needsNewSession
	desiredID := usablePreviousSessionID(s.sessionID)
	s.mu.Unlock()

	activeID, _, _, _ := s.rpc.sessionState()
	setupCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()

	if needsNew || (desiredID == "" && activeID == "") {
		if activeID != "" {
			s.bestEffortCloseSession(activeID)
		}
		res, err := s.rpc.CallMap(setupCtx, "session/new", s.sessionParams(""))
		if err != nil {
			return err
		}
		sid := stringVal(res, "sessionId")
		if sid == "" {
			return fmt.Errorf("agy acp session/new returned empty sessionId")
		}
		s.mu.Lock()
		s.sessionID = sid
		s.needsNewSession = false
		s.mu.Unlock()
		s.rpc.setActiveSession(sid, res)
		return nil
	}

	if desiredID == "" {
		// Reuse active session on this pooled process.
		s.mu.Lock()
		s.sessionID = activeID
		s.mu.Unlock()
		return nil
	}

	if desiredID == activeID {
		return nil
	}

	if activeID != "" {
		s.bestEffortCloseSession(activeID)
	}

	res, err := s.rpc.CallMap(setupCtx, "session/resume", s.sessionParams(desiredID))
	if err != nil {
		// Fall back to `session/load` before declaring the session missing.
		loadRes, loadErr := s.rpc.CallMap(setupCtx, "session/load", s.sessionParams(desiredID))
		if loadErr != nil {
			if isMissingSessionError(err) || isMissingSessionError(loadErr) {
				return staleSessionError{cause: err}
			}
			return err
		}
		res = loadRes
	}
	sid := stringVal(res, "sessionId")
	if sid == "" {
		sid = desiredID
	}
	s.mu.Lock()
	s.sessionID = sid
	s.needsNewSession = false
	s.mu.Unlock()
	s.rpc.setActiveSession(sid, res)
	return nil
}

func (s *persistentSession) bestEffortCloseSession(sessionID string) {
	if sessionID == "" {
		return
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), closeSessionWait)
	defer cancel()
	_, _ = s.rpc.Call(closeCtx, "session/close", record{"sessionId": sessionID})
	s.rpc.clearActiveSession()
}

func (s *persistentSession) sessionParams(sessionID string) record {
	cwd := strings.TrimSpace(s.req.Workspace)
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		} else {
			cwd = "."
		}
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	params := record{
		"cwd":        cwd,
		"mcpServers": []any{},
	}
	if sessionID != "" {
		params["sessionId"] = sessionID
	}
	if extraDir := extraInstructionWorkspace(s.req); extraDir != "" {
		if absDir, err := filepath.Abs(extraDir); err == nil {
			extraDir = absDir
		}
		params["additionalDirectories"] = []string{extraDir}
	}
	if prompt := s.rt.composeSystemPrompt(s.req); prompt != "" {
		params["_meta"] = record{
			"antigravity": record{
				"systemPrompt": record{
					"append": prompt,
				},
			},
		}
	}
	return params
}

func (s *persistentSession) reconcileConfig(ctx context.Context) error {
	sid, currentModel, currentMode, opts := s.rpc.sessionState()
	if sid == "" {
		return nil
	}

	// 1. Permission mode (`mode` config option: default, accept-edits, auto, plan).
	targetMode := s.targetSessionMode()
	if modeOpt, ok := optionByID(opts, "mode"); ok {
		if matched, ok := modeOpt.matchValue(targetMode); ok && matched != modeOpt.CurrentValue {
			res, err := s.rpc.CallMap(ctx, "session/set_config_option", record{
				"sessionId": sid,
				"configId":  "mode",
				"value":     matched,
			})
			if err != nil {
				logWarn("set mode config option failed", "mode", matched, "error", err)
			} else {
				s.rpc.updateConfigFromResult(res)
			}
		}
	} else if targetMode != "" && targetMode != currentMode {
		if res, err := s.rpc.CallMap(ctx, "session/set_mode", record{
			"sessionId": sid,
			"modeId":    targetMode,
		}); err == nil {
			s.rpc.updateConfigFromResult(res)
		}
	}

	// 2. Model (`model` config option or `session/set_model`).
	_, currentModel, _, opts = s.rpc.sessionState()
	var availableModels []string
	if modelOpt, ok := optionByID(opts, "model"); ok {
		availableModels = modelOpt.values()
	}
	wantModel, modelEffort := splitModelEffort(s.req.Model, availableModels)
	if wantModel != "" {
		if modelOpt, ok := optionByID(opts, "model"); ok {
			if matched, ok := modelOpt.matchValue(wantModel); ok {
				if matched != modelOpt.CurrentValue {
					res, err := s.rpc.CallMap(ctx, "session/set_config_option", record{
						"sessionId": sid,
						"configId":  "model",
						"value":     matched,
					})
					if err != nil {
						return fmt.Errorf("set model %q: %w", matched, err)
					}
					s.rpc.updateConfigFromResult(res)
				}
			} else if wantModel != currentModel {
				res, err := s.rpc.CallMap(ctx, "session/set_model", record{
					"sessionId": sid,
					"modelId":   wantModel,
				})
				if err != nil {
					s.emitNote(fmt.Sprintf("Model %q is not available in Antigravity; keeping %s.", wantModel, firstNonEmpty(modelOpt.CurrentValue, currentModel, "default")))
				} else {
					s.rpc.updateConfigFromResult(res)
				}
			}
		} else if wantModel != currentModel {
			if res, err := s.rpc.CallMap(ctx, "session/set_model", record{
				"sessionId": sid,
				"modelId":   wantModel,
			}); err == nil {
				s.rpc.updateConfigFromResult(res)
			}
		}
	}

	// 3. Thought level (`thought_level` config option — options depend on selected model).
	_, _, _, opts = s.rpc.sessionState()
	if thoughtOpt, ok := optionByID(opts, "thought_level"); ok {
		targetThought := resolveThoughtLevel(s.req.Effort, modelEffort, thoughtOpt.values())
		if targetThought != "" && !strings.EqualFold(targetThought, thoughtOpt.CurrentValue) {
			res, err := s.rpc.CallMap(ctx, "session/set_config_option", record{
				"sessionId": sid,
				"configId":  "thought_level",
				"value":     targetThought,
			})
			if err != nil {
				logWarn("set thought_level config option failed", "thought_level", targetThought, "error", err)
			} else {
				s.rpc.updateConfigFromResult(res)
			}
		}
	}
	return nil
}

func (s *persistentSession) targetSessionMode() string {
	if strings.EqualFold(strings.TrimSpace(s.req.PermissionMode), "plan") {
		return "plan"
	}
	if s.req.YoloMode {
		return "auto"
	}
	return normalizePermissionMode(s.rt.opts.PermissionMode)
}

func buildPromptBlocks(input runtime.Input) []any {
	text := input.RenderedPrompt()
	if strings.TrimSpace(text) == "" {
		text = "Please continue."
	}
	blocks := []any{
		record{
			"type": "text",
			"text": text,
		},
	}
	for _, att := range input.Attachments {
		isImage := att.Kind == "image" || strings.HasPrefix(strings.ToLower(strings.TrimSpace(att.ContentType)), "image/")
		if !isImage || strings.TrimSpace(att.LocalPath) == "" {
			continue
		}
		data, err := os.ReadFile(att.LocalPath)
		if err != nil {
			logWarn("read image attachment failed", "path", att.LocalPath, "error", err)
			continue
		}
		mimeType := strings.TrimSpace(att.ContentType)
		if mimeType == "" {
			mimeType = mime.TypeByExtension(filepath.Ext(att.LocalPath))
		}
		if idx := strings.Index(mimeType, ";"); idx > 0 {
			mimeType = strings.TrimSpace(mimeType[:idx])
		}
		if mimeType == "" {
			mimeType = "image/png"
		}
		blocks = append(blocks, record{
			"type":     "image",
			"data":     base64.StdEncoding.EncodeToString(data),
			"mimeType": mimeType,
		})
	}
	return blocks
}

func (s *persistentSession) formatTurnError(err error) string {
	if errors.Is(err, procpool.ErrProcessDead) {
		s.rt.pool.Detach(s.process)
		s.process.Kill()
		return agyExitedError(s.process.Stderr())
	}
	return err.Error()
}

func (s *persistentSession) setRunning(running bool) {
	s.mu.Lock()
	s.running = running
	s.mu.Unlock()
}

func (s *persistentSession) wasInterrupted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interruptRequested
}

func (s *persistentSession) emit(evt runtime.Event) {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	if s.events == nil {
		return
	}
	select {
	case <-s.done:
		return
	default:
	}
	if evt.Type == runtime.EventDelta {
		select {
		case s.events <- evt:
		default:
		}
		return
	}
	// Terminal and input-request events must not be dropped if delta buffer is full.
	select {
	case s.events <- evt:
	case <-s.done:
	}
}

func (s *persistentSession) emitFailed(errText string, stale bool) {
	s.emit(runtime.Event{Type: runtime.EventFailed, Error: errText, StaleSession: stale})
	s.mu.Lock()
	s.alive = false
	s.mu.Unlock()
	s.closeEventStream()
}

func (s *persistentSession) emitNote(text string) {
	if text == "" {
		return
	}
	s.emit(runtime.Event{
		Type: runtime.EventDelta,
		Process: []runtime.ProcessItem{{
			Type: "thinking",
			Text: text,
		}},
	})
}

func (s *persistentSession) closeEventStream() {
	s.closeOnce.Do(func() {
		s.eventMu.Lock()
		defer s.eventMu.Unlock()
		if s.done != nil {
			close(s.done)
		}
		if s.events != nil {
			close(s.events)
		}
	})
}

func (s *persistentSession) releaseTurn() {
	s.mu.Lock()
	held := s.turnHeld
	s.turnHeld = false
	s.mu.Unlock()
	if held {
		s.process.ReleaseTurn()
	}
}
