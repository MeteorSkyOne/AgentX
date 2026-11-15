package agy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

const (
	defaultBrainPollInterval          = 100 * time.Millisecond
	defaultBackgroundTurnClaimTimeout = 30 * time.Second
	defaultBackgroundTurnIdleTimeout  = 5 * time.Minute
)

var errNoBackgroundTurn = errors.New("no background turn to adopt")

type brainMonitorKey struct{}

func getBrainMonitor(proc *procpool.ManagedProcess, key string, rt *Runtime, env []string) *brainMonitor {
	proc.Mu.Lock()
	defer proc.Mu.Unlock()
	if v, ok := proc.UserData[brainMonitorKey{}]; ok {
		return v.(*brainMonitor)
	}
	appDataDir := agyAppDataDir(env, rt.opts.ExtraArgs)
	reg := newTaskRegistry(key, appDataDir, rt.reportBackgroundTasks)
	mon := newBrainMonitor(proc, key, rt, appDataDir, reg)
	if proc.UserData == nil {
		proc.UserData = make(map[any]any)
	}
	proc.UserData[brainMonitorKey{}] = mon
	return mon
}

func monitorOf(proc *procpool.ManagedProcess) *brainMonitor {
	if proc == nil {
		return nil
	}
	proc.Mu.Lock()
	defer proc.Mu.Unlock()
	m, _ := proc.UserData[brainMonitorKey{}].(*brainMonitor)
	return m
}

type bgTurnStream struct {
	mu          sync.Mutex
	steps       []transcriptStep
	notify      chan struct{}
	inputClosed bool
}

func newBgTurnStream() *bgTurnStream {
	return &bgTurnStream{
		notify: make(chan struct{}, 1),
	}
}

func (b *bgTurnStream) push(step transcriptStep) bool {
	b.mu.Lock()
	if b.inputClosed {
		b.mu.Unlock()
		return false
	}
	b.steps = append(b.steps, step)
	if isTerminalBackgroundStep(step) {
		b.inputClosed = true
	}
	b.mu.Unlock()
	select {
	case b.notify <- struct{}{}:
	default:
	}
	return true
}

func (b *bgTurnStream) isClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.inputClosed
}

func (b *bgTurnStream) closeInput() {
	b.mu.Lock()
	b.inputClosed = true
	b.mu.Unlock()
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

func (b *bgTurnStream) drain() (steps []transcriptStep, closed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.steps) > 0 {
		steps = make([]transcriptStep, len(b.steps))
		copy(steps, b.steps)
		b.steps = nil
	}
	return steps, b.inputClosed && len(b.steps) == 0
}

// brainMonitor tails Antigravity's per-session brain directory
// (`<appDataDir>/brain/<sessionId>/.system_generated/...`) for one pooled
// `agy acp` process.
//
// `agy acp` stops forwarding `session/update` notifications once a
// `session/prompt` request returns, so when a background command (`task-*`),
// subagent, or schedule wakes the agent up after its reply ended, the wakeup
// turn runs silently inside `agy` and writes its steps only to
// `transcript_full.jsonl`. Monitoring the brain directory both tracks live
// background tasks (`runtime.BackgroundTaskReporter`) and claims/streams those
// unprompted wakeup turns (`runtime.BackgroundTurnNotifier`).
type brainMonitor struct {
	process      *procpool.ManagedProcess
	key          string
	rt           *Runtime
	appDataDir   string
	registry     *taskRegistry
	pollInterval time.Duration
	claimTimeout time.Duration

	mu                  sync.Mutex
	sessionID           string
	transcriptPath      string
	offset              int64
	lastStepIndex       int
	seenMessages        map[string]struct{}
	foregroundActive    bool
	discardUntilTurnEnd bool
	bgTurn              *bgTurnStream
	claimExpiresAt      time.Time
	pendingSteps        []transcriptStep
}

func newBrainMonitor(proc *procpool.ManagedProcess, key string, rt *Runtime, appDataDir string, reg *taskRegistry) *brainMonitor {
	pollInterval := defaultBrainPollInterval
	claimTimeout := defaultBackgroundTurnClaimTimeout
	if rt != nil {
		if rt.opts.pollInterval > 0 {
			pollInterval = rt.opts.pollInterval
		}
		if rt.opts.claimTimeout > 0 {
			claimTimeout = rt.opts.claimTimeout
		}
	}
	m := &brainMonitor{
		process:       proc,
		key:           key,
		rt:            rt,
		appDataDir:    appDataDir,
		registry:      reg,
		pollInterval:  pollInterval,
		claimTimeout:  claimTimeout,
		lastStepIndex: -1,
		seenMessages:  make(map[string]struct{}),
	}
	go m.loop()
	return m
}

func (m *brainMonitor) loop() {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.process.Done():
			m.mu.Lock()
			bt := m.bgTurn
			m.mu.Unlock()
			if bt != nil {
				bt.closeInput()
			}
			m.registry.processExited()
			return
		case <-ticker.C:
			m.poll()
		}
	}
}

func (m *brainMonitor) activeSessionID() string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionID
}

// activateSession binds the monitor to an ACP session ID. When `resumed` is
// true (`session/resume` or `session/load`), existing transcript lines and
// message files belong to prior history and are skipped so they are not
// replayed as fresh background turns.
func (m *brainMonitor) activateSession(sessionID string, resumed bool) {
	if m == nil || sessionID == "" {
		return
	}
	m.mu.Lock()
	if m.sessionID == sessionID {
		m.mu.Unlock()
		return
	}
	m.sessionID = sessionID
	m.transcriptPath = ""
	m.offset = 0
	m.lastStepIndex = -1
	m.seenMessages = make(map[string]struct{})
	m.pendingSteps = nil
	m.discardUntilTurnEnd = false

	if resumed && m.appDataDir != "" {
		if path := m.resolveTranscriptPathLocked(); path != "" {
			if info, err := os.Stat(path); err == nil {
				m.offset = info.Size()
				m.lastStepIndex = readLastStepIndex(path)
			}
		}
		for _, name := range m.listMessageFilesLocked() {
			m.seenMessages[name] = struct{}{}
		}
	}
	m.mu.Unlock()

	m.registry.setSession(sessionID)
}

func (m *brainMonitor) closeSession() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.sessionID = ""
	m.transcriptPath = ""
	m.offset = 0
	m.lastStepIndex = -1
	m.seenMessages = make(map[string]struct{})
	m.pendingSteps = nil
	m.discardUntilTurnEnd = false
	if m.bgTurn != nil {
		m.bgTurn.closeInput()
		m.bgTurn = nil
	}
	m.claimExpiresAt = time.Time{}
	m.mu.Unlock()

	m.registry.closeSession()
}

func (m *brainMonitor) beginForegroundTurn() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.foregroundActive = true
	m.discardUntilTurnEnd = false
	m.mu.Unlock()
}

// endForegroundTurn flushes all transcript steps and message files produced
// during the active `session/prompt` before the caller releases `turnMu`, so
// steps already streamed over ACP stdout are never mistaken for an unprompted
// background wakeup.
func (m *brainMonitor) endForegroundTurn() {
	if m == nil {
		return
	}
	m.mu.Lock()
	_, _ = m.pollLocked(true)
	m.foregroundActive = false
	m.mu.Unlock()
}

func (m *brainMonitor) poll() {
	if m == nil {
		return
	}
	if m.registry.hasRunning() {
		m.process.Touch()
	}

	m.mu.Lock()
	offer, expiredClaim := m.pollLocked(false)
	m.mu.Unlock()

	if expiredClaim {
		slog.Warn("agy: claimed background turn was never adopted; releasing it", "key", m.key)
		m.process.AbandonClaim()
	}
	if offer {
		if m.rt.startBackgroundTurn(m.key) {
			slog.Info("agy: agent started a background turn on its own", "key", m.key)
		} else {
			m.abandonUnadoptedClaim()
		}
	}
}

func (m *brainMonitor) pollLocked(foregroundFlush bool) (offerTurn bool, expiredClaim bool) {
	if m.sessionID == "" || m.appDataDir == "" {
		return false, false
	}

	if !m.claimExpiresAt.IsZero() && !time.Now().Before(m.claimExpiresAt) {
		m.claimExpiresAt = time.Time{}
		if m.bgTurn != nil {
			if !m.bgTurn.isClosed() {
				m.discardUntilTurnEnd = true
			}
			m.bgTurn.closeInput()
			m.bgTurn = nil
		}
		expiredClaim = true
	}

	m.scanMessageFilesLocked()

	steps := m.readNewStepsLocked()
	for _, step := range steps {
		m.registry.observeStep(step)
	}

	if foregroundFlush || m.foregroundActive {
		return false, expiredClaim
	}

	if len(m.pendingSteps) > 0 {
		combined := make([]transcriptStep, 0, len(m.pendingSteps)+len(steps))
		combined = append(combined, m.pendingSteps...)
		combined = append(combined, steps...)
		m.pendingSteps = nil
		steps = combined
	}

	for i, step := range steps {
		if m.bgTurn != nil {
			if m.bgTurn.push(step) {
				continue
			}
			// Current background turn already received its terminal step, but
			// its session goroutine has not released `turnMu` yet. Queue the
			// remaining steps for the next background turn.
			m.pendingSteps = append(m.pendingSteps, steps[i:]...)
			break
		}

		if m.discardUntilTurnEnd {
			if isTerminalBackgroundStep(step) {
				m.discardUntilTurnEnd = false
			}
			continue
		}

		if !startsBackgroundTurn(step) {
			continue
		}
		if !m.process.ClaimTurn(m.claimTimeout) {
			// Another turn holds `turnMu`; hold steps until the turn releases.
			m.pendingSteps = append(m.pendingSteps, steps[i:]...)
			break
		}
		stream := newBgTurnStream()
		stream.push(step)
		m.bgTurn = stream
		m.claimExpiresAt = time.Now().Add(m.claimTimeout)
		offerTurn = true
	}

	return offerTurn, expiredClaim
}

func (m *brainMonitor) adoptTurn() *bgTurnStream {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bgTurn == nil {
		return nil
	}
	if !m.process.AdoptClaim() {
		if !m.bgTurn.isClosed() {
			m.discardUntilTurnEnd = true
		}
		m.bgTurn.closeInput()
		m.bgTurn = nil
		m.claimExpiresAt = time.Time{}
		return nil
	}
	m.claimExpiresAt = time.Time{}
	return m.bgTurn
}

func (m *brainMonitor) abandonUnadoptedClaim() {
	if m == nil {
		return
	}
	m.mu.Lock()
	hadClaim := !m.claimExpiresAt.IsZero()
	m.claimExpiresAt = time.Time{}
	if hadClaim && m.bgTurn != nil {
		if !m.bgTurn.isClosed() {
			m.discardUntilTurnEnd = true
		}
		m.bgTurn.closeInput()
		m.bgTurn = nil
	}
	m.mu.Unlock()
	if hadClaim {
		m.process.AbandonClaim()
	}
}

func (m *brainMonitor) finishBackgroundTurn(stream *bgTurnStream) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.bgTurn == stream {
		m.bgTurn = nil
		m.claimExpiresAt = time.Time{}
	}
	m.mu.Unlock()
}

func (m *brainMonitor) kickPending() {
	if m == nil {
		return
	}
	m.mu.Lock()
	hasPending := len(m.pendingSteps) > 0
	m.mu.Unlock()
	if hasPending {
		m.poll()
	}
}

func (m *brainMonitor) resolveTranscriptPathLocked() string {
	if m.transcriptPath != "" {
		if _, err := os.Stat(m.transcriptPath); err == nil {
			return m.transcriptPath
		}
	}
	logsDir := filepath.Join(m.appDataDir, "brain", m.sessionID, ".system_generated", "logs")
	fullPath := filepath.Join(logsDir, "transcript_full.jsonl")
	if _, err := os.Stat(fullPath); err == nil {
		m.transcriptPath = fullPath
		return fullPath
	}
	compactPath := filepath.Join(logsDir, "transcript.jsonl")
	if _, err := os.Stat(compactPath); err == nil {
		m.transcriptPath = compactPath
		return compactPath
	}
	return ""
}

func (m *brainMonitor) readNewStepsLocked() []transcriptStep {
	path := m.resolveTranscriptPathLocked()
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if info.Size() < m.offset {
		m.offset = 0
	}
	if info.Size() == m.offset {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	if _, err := f.Seek(m.offset, io.SeekStart); err != nil {
		return nil
	}
	chunk, err := io.ReadAll(f)
	if err != nil || len(chunk) == 0 {
		return nil
	}
	lastNL := bytes.LastIndexByte(chunk, '\n')
	if lastNL < 0 {
		// Incomplete line write in progress; wait for trailing newline.
		return nil
	}
	complete := chunk[:lastNL+1]
	m.offset += int64(len(complete))

	var steps []transcriptStep
	for _, rawLine := range bytes.Split(complete, []byte{'\n'}) {
		rawLine = bytes.TrimSpace(rawLine)
		if len(rawLine) == 0 {
			continue
		}
		var step transcriptStep
		if err := json.Unmarshal(rawLine, &step); err != nil {
			continue
		}
		if step.StepIndex <= m.lastStepIndex {
			continue
		}
		m.lastStepIndex = step.StepIndex
		steps = append(steps, step)
	}
	return steps
}

func (m *brainMonitor) listMessageFilesLocked() []string {
	msgDir := filepath.Join(m.appDataDir, "brain", m.sessionID, ".system_generated", "messages")
	entries, err := os.ReadDir(msgDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || name == "read.json" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m *brainMonitor) scanMessageFilesLocked() {
	msgDir := filepath.Join(m.appDataDir, "brain", m.sessionID, ".system_generated", "messages")
	for _, name := range m.listMessageFilesLocked() {
		if _, seen := m.seenMessages[name]; seen {
			continue
		}
		data, err := os.ReadFile(filepath.Join(msgDir, name))
		if err != nil || len(bytes.TrimSpace(data)) == 0 {
			continue
		}
		var parsed agyMessageFile
		if err := json.Unmarshal(data, &parsed); err != nil {
			// Might still be mid-write; retry next poll.
			continue
		}
		m.seenMessages[name] = struct{}{}
		m.registry.observeMessageFile(parsed)
	}
}

func readLastStepIndex(path string) int {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return -1
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var step transcriptStep
		if json.Unmarshal(line, &step) == nil {
			return step.StepIndex
		}
	}
	return -1
}

// startsBackgroundTurn reports whether a transcript step appended while idle
// opens an unprompted reactive wakeup turn. Low-priority system notices (such
// as restart notices or `manage_task` cancellations) remain queued in
// `undelivered/` and do not wake the planner on their own.
func startsBackgroundTurn(step transcriptStep) bool {
	switch step.Type {
	case "SYSTEM_MESSAGE":
		return !strings.Contains(step.Content, "priority=MESSAGE_PRIORITY_LOW")
	case "PLANNER_RESPONSE":
		return true
	default:
		return false
	}
}

func isTerminalBackgroundStep(step transcriptStep) bool {
	if step.Type != "PLANNER_RESPONSE" {
		return false
	}
	return len(step.ToolCalls) == 0
}

type trackedBgCall struct {
	callID     string
	name       string
	input      any
	isSubagent bool
}

// AdoptBackgroundTurn implements runtime.BackgroundTurnAdopter, reading an
// unprompted wakeup turn claimed by `brainMonitor` in place of `Send`.
func (s *persistentSession) AdoptBackgroundTurn(ctx context.Context) error {
	s.mu.Lock()
	if !s.alive {
		s.mu.Unlock()
		return procpool.ErrProcessDead
	}
	if s.started {
		s.mu.Unlock()
		return errors.New("session already started")
	}
	s.started = true
	s.mu.Unlock()

	if s.monitor == nil {
		return errNoBackgroundTurn
	}
	s.process.AttachReader()
	stream := s.monitor.adoptTurn()
	if stream == nil {
		s.process.DetachReader()
		return errNoBackgroundTurn
	}

	activeID, _, _, _ := s.rpc.sessionState()
	if activeID == "" {
		activeID = s.monitor.activeSessionID()
	}

	s.mu.Lock()
	s.turnHeld = true
	if activeID != "" {
		s.sessionID = activeID
		s.needsNewSession = false
	}
	s.mu.Unlock()

	go s.runBackgroundTurn(ctx, stream)
	return nil
}

func (s *persistentSession) runBackgroundTurn(ctx context.Context, stream *bgTurnStream) {
	defer func() {
		s.setRunning(false)
		s.process.DetachReader()
		if s.monitor != nil {
			s.monitor.finishBackgroundTurn(stream)
		}
		s.releaseTurn()
		s.mu.Lock()
		s.alive = false
		s.mu.Unlock()
		s.closeEventStream()
		if s.monitor != nil {
			s.monitor.kickPending()
		}
	}()

	started := time.Now()
	sid := s.CurrentSessionID()
	state := newTurnState("")
	state.contextUsage = s.rpc.contextUsage()

	bgCtx, cancelBg := context.WithCancel(ctx)
	defer cancelBg()

	s.setRunning(true)
	if s.wasInterrupted() {
		if sid != "" {
			_ = s.rpc.Notify("session/cancel", record{"sessionId": sid})
		}
		s.emit(runtime.Event{
			Type:  runtime.EventCanceled,
			Usage: state.timer.ApplyAt(nil, time.Now().UTC()),
		})
		return
	}

	var (
		finalText       strings.Builder
		modelID         string
		inputTokens     int64
		cacheReadTokens int64
		outputTokens    int64
		hasTokens       bool
		pendingCalls    []trackedBgCall
	)

	idleTimer := time.NewTimer(defaultBackgroundTurnIdleTimeout)
	defer idleTimer.Stop()
	resetIdle := func() {
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(defaultBackgroundTurnIdleTimeout)
	}

	processStep := func(step transcriptStep) bool {
		now := time.Now().UTC()
		switch step.Type {
		case "PLANNER_RESPONSE":
			if m := strings.TrimSpace(step.Model); m != "" {
				modelID = m
			}
			if step.InputTokens > 0 || step.CacheTokens > 0 || step.OutputTokens > 0 {
				hasTokens = true
				inputTokens += step.InputTokens
				cacheReadTokens += step.CacheTokens
				outputTokens += step.OutputTokens
			}

			if thought := strings.TrimSpace(step.Thinking); thought != "" {
				state.timer.ObserveStreamAt(now)
				s.emit(runtime.Event{
					Type:     runtime.EventDelta,
					Thinking: thought,
					Process: []runtime.ProcessItem{{
						Type: "thinking",
						Text: thought,
					}},
				})
			}

			if len(step.ToolCalls) > 0 {
				if pre := strings.TrimSpace(step.Content); pre != "" {
					state.timer.ObserveStreamAt(now)
					s.emit(runtime.Event{
						Type:      runtime.EventDelta,
						Thinking:  pre,
						ClearText: true,
						Process: []runtime.ProcessItem{{
							Type: "thinking",
							Text: pre,
						}},
					})
				}
				var items []runtime.ProcessItem
				for i, tc := range step.ToolCalls {
					args := normalizeToolArgs(tc.Args)
					callID := fmtStepCallID(step.StepIndex, i, tc.ID)
					state.timer.ToolStartedAt(callID, now)
					isSub := tc.Name == "invoke_subagent"
					items = append(items, runtime.ProcessItem{
						Type:       "tool_call",
						ToolName:   tc.Name,
						ToolCallID: callID,
						Status:     "in_progress",
						Input:      args,
					})
					if isSub {
						items = append(items, runtime.ProcessItem{
							Type:       "subagent_started",
							ToolCallID: callID,
						})
					}
					pendingCalls = append(pendingCalls, trackedBgCall{
						callID:     callID,
						name:       tc.Name,
						input:      args,
						isSubagent: isSub,
					})
				}
				if len(items) > 0 {
					s.emit(runtime.Event{
						Type:    runtime.EventDelta,
						Process: items,
					})
				}
				return false
			}

			// Terminal PLANNER_RESPONSE (no tool_calls).
			completedAt := time.Now().UTC()
			upperStatus := strings.ToUpper(strings.TrimSpace(step.Status))
			if upperStatus == "CANCELED" || upperStatus == "CANCELLED" {
				s.emit(runtime.Event{
					Type:  runtime.EventCanceled,
					Usage: state.timer.ApplyAt(nil, completedAt),
				})
				return true
			}
			if upperStatus == "ERROR" {
				s.emit(runtime.Event{
					Type:  runtime.EventFailed,
					Error: firstNonEmpty(strings.TrimSpace(step.Content), "antigravity background turn failed"),
					Usage: state.timer.ApplyAt(nil, completedAt),
				})
				return true
			}

			if step.Content != "" {
				state.timer.ObserveStreamAt(now)
				finalText.WriteString(step.Content)
				s.emit(runtime.Event{
					Type: runtime.EventDelta,
					Text: step.Content,
				})
			}

			if modelID == "" {
				_, activeModel, _, _ := s.rpc.sessionState()
				modelID = firstNonEmpty(activeModel, strings.TrimSpace(s.req.Model))
			}
			var rawUsage record
			if hasTokens {
				rawUsage = record{
					"inputTokens":      inputTokens,
					"cachedReadTokens": cacheReadTokens,
					"outputTokens":     outputTokens,
					"totalTokens":      inputTokens + cacheReadTokens + outputTokens,
				}
			}
			ctxUsage := state.contextUsage
			if ctxUsage == nil {
				ctxUsage = s.rpc.contextUsage()
			}
			var promptPayload record
			if rawUsage != nil {
				promptPayload = record{"usage": rawUsage}
			}
			usage := state.timer.ApplyAt(
				buildUsage(promptPayload, modelID, completedAt.Sub(started).Milliseconds(), ctxUsage),
				completedAt,
			)
			s.emit(runtime.Event{
				Type:  runtime.EventCompleted,
				Text:  finalText.String(),
				Usage: usage,
			})
			return true

		case "GENERIC":
			if len(pendingCalls) > 0 {
				call := pendingCalls[0]
				pendingCalls = pendingCalls[1:]
				state.timer.ToolCompletedAt(call.callID, now)
				normStatus := "completed"
				switch strings.ToUpper(strings.TrimSpace(step.Status)) {
				case "ERROR", "FAILED":
					normStatus = "failed"
				case "CANCELED", "CANCELLED":
					normStatus = "canceled"
				}
				output := cleanTranscriptToolOutput(step.Content)
				items := []runtime.ProcessItem{{
					Type:       "tool_result",
					ToolName:   call.name,
					ToolCallID: call.callID,
					Status:     normStatus,
					Input:      call.input,
					Output:     output,
				}}
				if call.isSubagent {
					subStatus := "completed"
					if normStatus == "failed" {
						subStatus = "failed"
					}
					items = append(items, runtime.ProcessItem{
						Type:       "subagent_completed",
						ToolCallID: call.callID,
						Status:     subStatus,
					})
				}
				s.emit(runtime.Event{
					Type:    runtime.EventDelta,
					Process: items,
				})
			}
		}
		return false
	}

	for {
		steps, closed := stream.drain()
		for _, step := range steps {
			resetIdle()
			if processStep(step) {
				return
			}
		}
		if closed {
			// Process exited before the planner emitted a terminal response.
			s.emit(runtime.Event{
				Type:  runtime.EventFailed,
				Error: s.formatTurnError(procpool.ErrProcessDead),
				Usage: state.timer.ApplyAt(nil, time.Now().UTC()),
			})
			return
		}

		select {
		case <-stream.notify:
		case msg, ok := <-s.rpc.Messages():
			if !ok {
				s.emit(runtime.Event{
					Type:  runtime.EventFailed,
					Error: s.formatTurnError(procpool.ErrProcessDead),
					Usage: state.timer.ApplyAt(nil, time.Now().UTC()),
				})
				return
			}
			if msg.Method == "session/request_permission" && msg.IDPresent {
				resetIdle()
				s.handlePermissionRequest(bgCtx, state, msg.ID, msg.Params)
			} else if msg.IDPresent {
				_ = s.rpc.RespondError(msg.ID, -32601, "Method not found: "+msg.Method)
			}
		case <-s.interruptCh:
			if sid != "" {
				_ = s.rpc.Notify("session/cancel", record{"sessionId": sid})
			}
			s.emit(runtime.Event{
				Type:  runtime.EventCanceled,
				Usage: state.timer.ApplyAt(nil, time.Now().UTC()),
			})
			return
		case <-ctx.Done():
			if sid != "" {
				_ = s.rpc.Notify("session/cancel", record{"sessionId": sid})
			}
			s.emit(runtime.Event{
				Type:  runtime.EventCanceled,
				Usage: state.timer.ApplyAt(nil, time.Now().UTC()),
			})
			return
		case <-idleTimer.C:
			s.emit(runtime.Event{
				Type:  runtime.EventFailed,
				Error: "antigravity background turn timed out waiting for response",
				Usage: state.timer.ApplyAt(nil, time.Now().UTC()),
			})
			return
		}
	}
}
