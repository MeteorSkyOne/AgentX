package pi

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

const (
	statsTimeout   = 10 * time.Second
	compactTimeout = 10 * time.Minute
)

type inputAnswer struct {
	questionID string
	answer     string
}

type sessionRPC interface {
	Call(ctx context.Context, cmd record) (any, error)
	CallMap(ctx context.Context, cmd record) (record, error)
	Send(msg record) error
	Events() <-chan record
}

type persistentSession struct {
	process *procpool.ManagedProcess
	rpc     sessionRPC
	key     string
	rt      *Runtime
	req     runtime.StartSessionRequest
	events  chan runtime.Event

	mu        sync.Mutex
	eventMu   sync.Mutex
	sessionID string
	model     string
	// needsNewSession is set when the pooled process still holds the previous
	// conversation but the stored provider session was cleared (/new).
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
}

func newPersistentSession(proc *procpool.ManagedProcess, rpc sessionRPC, key string, rt *Runtime, req runtime.StartSessionRequest) *persistentSession {
	return &persistentSession{
		process:      proc,
		rpc:          rpc,
		key:          key,
		rt:           rt,
		req:          req,
		events:       make(chan runtime.Event, 64),
		sessionID:    "pi:" + key,
		alive:        true,
		interruptCh:  make(chan struct{}),
		done:         make(chan struct{}),
		pendingInput: make(chan inputAnswer, 1),
	}
}

// waitReady blocks until a freshly started pi process answers get_state, which
// also confirms the session id and model pi actually selected.
func (s *persistentSession) waitReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	state, err := s.rpc.CallMap(ctx, record{"type": "get_state"})
	if err != nil {
		if stderr := strings.TrimSpace(s.process.Stderr()); stderr != "" {
			return fmt.Errorf("pi failed to start: %s", truncateDetail(stderr))
		}
		return fmt.Errorf("pi failed to start: %w", err)
	}
	s.applyState(state)
	return nil
}

func (s *persistentSession) applyState(state record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sid := stringVal(state, "sessionId"); sid != "" {
		s.sessionID = sid
	}
	if model := mapVal(state, "model"); model != nil {
		s.model = stringVal(model, "id")
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
		return fmt.Errorf("no pending input request")
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

// Interrupt aborts the in-flight prompt. pi answers `abort` once the session is
// idle again and then emits agent_settled with aborted=true, which ends the
// turn as canceled.
func (s *persistentSession) Interrupt(ctx context.Context) error {
	s.mu.Lock()
	if !s.alive {
		s.mu.Unlock()
		return procpool.ErrProcessDead
	}
	s.interruptRequested = true
	running := s.running
	s.mu.Unlock()
	s.interruptOnce.Do(func() { close(s.interruptCh) })
	if !running {
		return nil
	}
	_, err := s.rpc.Call(ctx, record{"type": "abort"})
	return err
}

// Steer queues a message for delivery after the current assistant turn
// finishes its tool calls, before the next model call.
func (s *persistentSession) Steer(ctx context.Context, input runtime.Input) error {
	if !s.CanSteer() {
		return errNoActiveTurn
	}
	cmd := record{"type": "steer", "message": input.RenderedPrompt()}
	if images := imageContents(input.Attachments); len(images) > 0 {
		cmd["images"] = images
	}
	_, err := s.rpc.Call(ctx, cmd)
	return err
}

func (s *persistentSession) CanSteer() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alive && s.running
}

// ContextUsage reads the current context-window fill from pi without starting
// a turn.
func (s *persistentSession) ContextUsage(ctx context.Context) (*runtime.ContextUsage, error) {
	s.mu.Lock()
	alive := s.alive
	model := s.model
	s.mu.Unlock()
	if !alive {
		return nil, procpool.ErrProcessDead
	}
	stats, err := s.rpc.CallMap(ctx, record{"type": "get_session_stats"})
	if err != nil {
		return nil, err
	}
	return contextUsageFromStats(stats, model), nil
}

func (s *persistentSession) setRunning(running bool) {
	s.mu.Lock()
	s.running = running
	s.mu.Unlock()
}

func (s *persistentSession) interrupted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interruptRequested
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

	if err := s.prepare(ctx); err != nil {
		s.emit(runtime.Event{Type: runtime.EventFailed, Error: err.Error()})
		return
	}

	if instructions, ok := parseCompactPrompt(input.Prompt); ok {
		s.runCompact(ctx, instructions)
		return
	}

	cmd := record{"type": "prompt", "message": input.RenderedPrompt()}
	if images := imageContents(input.Attachments); len(images) > 0 {
		cmd["images"] = images
	}
	s.setRunning(true)
	resp, err := s.rpc.CallMap(ctx, cmd)
	if err != nil {
		s.setRunning(false)
		s.emit(runtime.Event{Type: runtime.EventFailed, Error: err.Error()})
		return
	}
	if stringVal(resp, "disposition") == "handled" {
		// An extension command or input handler consumed the prompt; no agent
		// run follows, so there is no agent_settled to wait for.
		s.setRunning(false)
		s.emit(runtime.Event{Type: runtime.EventCompleted})
		return
	}
	if s.interrupted() {
		if _, err := s.rpc.Call(ctx, record{"type": "abort"}); err != nil {
			logWarn("abort after early interrupt failed", "error", err)
		}
	}
	s.processEvents(ctx)
}

// prepare brings the pooled process in line with the session request: start a
// fresh pi session when the context was reset, then apply the agent's model
// and effort when they differ from what the process currently uses.
func (s *persistentSession) prepare(ctx context.Context) error {
	s.mu.Lock()
	needsNew := s.needsNewSession
	s.needsNewSession = false
	s.mu.Unlock()

	if needsNew {
		if _, err := s.rpc.CallMap(ctx, record{"type": "new_session"}); err != nil {
			return fmt.Errorf("new_session: %w", err)
		}
	}
	state, err := s.rpc.CallMap(ctx, record{"type": "get_state"})
	if err != nil {
		return fmt.Errorf("get_state: %w", err)
	}
	s.applyState(state)

	current := modelRefFromRecord(mapVal(state, "model"))
	wantModel, modelThinking := splitModelThinking(s.req.Model)
	if wantModel != "" && !matchesModel(current, wantModel) {
		models, err := s.rpc.CallMap(ctx, record{"type": "get_available_models"})
		if err != nil {
			return fmt.Errorf("get_available_models: %w", err)
		}
		ref, ok := findModel(modelRefsFromData(models), wantModel)
		if !ok {
			s.emitNote(fmt.Sprintf("Model %q is not configured in pi; using %s/%s.", wantModel, current.Provider, current.ID))
		} else {
			data, err := s.rpc.CallMap(ctx, record{"type": "set_model", "provider": ref.Provider, "modelId": ref.ID})
			if err != nil {
				return fmt.Errorf("set_model: %w", err)
			}
			current = modelRefFromRecord(data)
			if current.ID == "" {
				current = ref
			}
			s.mu.Lock()
			s.model = current.ID
			s.mu.Unlock()
		}
	}

	if err := s.syncMode(ctx); err != nil {
		return err
	}

	level := thinkingLevelForEffort(s.req.Effort, modelThinking)
	if level != "" && level != stringVal(state, "thinkingLevel") {
		available, err := s.rpc.CallMap(ctx, record{"type": "get_available_thinking_levels"})
		if err != nil {
			return fmt.Errorf("get_available_thinking_levels: %w", err)
		}
		clamped := clampThinkingLevel(level, stringsFromData(available, "levels"))
		if clamped != stringVal(state, "thinkingLevel") {
			if _, err := s.rpc.Call(ctx, record{"type": "set_thinking_level", "level": clamped}); err != nil {
				return fmt.Errorf("set_thinking_level: %w", err)
			}
		}
	}
	return nil
}

// syncMode pushes the turn's plan/yolo mode to the bundled extension when it
// differs from what the process last received. Without the extension the
// command is not handled; that is logged once per process and otherwise ignored.
func (s *persistentSession) syncMode(ctx context.Context) error {
	if s.rt == nil || s.rt.extPath == "" {
		return nil
	}
	want := modeState{
		Plan: strings.EqualFold(strings.TrimSpace(s.req.PermissionMode), "plan"),
		Yolo: s.req.YoloMode,
	}
	if current, known := processMode(s.process); known && current == want {
		return nil
	}
	resp, err := s.rpc.CallMap(ctx, record{"type": "prompt", "message": modeCommand(want)})
	if err != nil {
		return fmt.Errorf("agentx-mode: %w", err)
	}
	if stringVal(resp, "disposition") != "handled" {
		logWarn("agentx extension did not handle /agentx-mode; plan and yolo modes are not enforced", "key", s.key)
	}
	setProcessMode(s.process, want)
	return nil
}

func modeCommand(mode modeState) string {
	onOff := func(v bool) string {
		if v {
			return "on"
		}
		return "off"
	}
	return "/agentx-mode plan=" + onOff(mode.Plan) + " yolo=" + onOff(mode.Yolo)
}

// runCompact handles the /compact slash command with pi's native compaction.
func (s *persistentSession) runCompact(ctx context.Context, instructions string) {
	ctx, cancel := context.WithTimeout(ctx, compactTimeout)
	defer cancel()
	cmd := record{"type": "compact"}
	if instructions != "" {
		cmd["customInstructions"] = instructions
	}
	started := time.Now()
	result, err := s.rpc.CallMap(ctx, cmd)
	if err != nil {
		var rpcErr *rpcError
		if errors.As(err, &rpcErr) && strings.Contains(strings.ToLower(rpcErr.Message), "nothing to compact") {
			// Not a failure: pi declines to compact a context that is still small.
			s.emit(runtime.Event{Type: runtime.EventCompleted, Text: rpcErr.Message})
			return
		}
		s.emit(runtime.Event{Type: runtime.EventFailed, Error: fmt.Sprintf("compact: %v", err)})
		return
	}
	var b strings.Builder
	b.WriteString("Context compacted")
	before := int64Ptr(result, "tokensBefore")
	after := int64Ptr(result, "estimatedTokensAfter")
	if before != nil && after != nil {
		fmt.Fprintf(&b, " (%d → ~%d tokens)", *before, *after)
	}
	b.WriteString(".")
	if summary := strings.TrimSpace(stringVal(result, "summary")); summary != "" {
		b.WriteString("\n\n")
		b.WriteString(summary)
	}
	usage := (&usageTotals{}).toRuntime(time.Since(started).Milliseconds(), s.fetchContextUsage(ctx))
	s.emit(runtime.Event{Type: runtime.EventCompleted, Text: b.String(), Usage: usage})
}

// parseCompactPrompt recognises the /compact slash command AgentX forwards as
// a prompt and returns its optional custom instructions.
func parseCompactPrompt(prompt string) (string, bool) {
	prompt = strings.TrimSpace(prompt)
	lower := strings.ToLower(prompt)
	if lower != "/compact" && !strings.HasPrefix(lower, "/compact ") {
		return "", false
	}
	return strings.TrimSpace(prompt[len("/compact"):]), true
}

// fetchContextUsage reads the post-turn context fill; failures are logged, not
// surfaced, because the turn itself already succeeded.
func (s *persistentSession) fetchContextUsage(ctx context.Context) *runtime.ContextUsage {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statsTimeout)
	defer cancel()
	usage, err := s.ContextUsage(ctx)
	if err != nil {
		logWarn("get_session_stats failed", "error", err)
		return nil
	}
	return usage
}

// imageContents converts image attachments into pi ImageContent blocks.
func imageContents(attachments []runtime.Attachment) []record {
	var images []record
	for _, att := range attachments {
		if att.Kind != "image" || att.LocalPath == "" {
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
		if mimeType == "" {
			mimeType = "image/png"
		}
		images = append(images, record{
			"type":     "image",
			"data":     base64.StdEncoding.EncodeToString(data),
			"mimeType": mimeType,
		})
	}
	return images
}
