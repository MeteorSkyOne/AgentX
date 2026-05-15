package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
)

const (
	defaultTerminalIdleTimeout             = 30 * time.Minute
	defaultTerminalMaxSessionsPerWorkspace = 8
	defaultTerminalReplayBytes             = 8 * 1024 * 1024
	defaultTerminalCols                    = 80
	defaultTerminalRows                    = 24
	maxTerminalInputBytes                  = 64 * 1024
	maxTerminalTitleRunes                  = 80
)

var (
	ErrTerminalNotFound = errors.New("terminal not found")
	ErrTerminalExited   = errors.New("terminal exited")
)

type TerminalOptions struct {
	Shell                   string
	IdleTimeout             time.Duration
	MaxSessionsPerWorkspace int
	ReplayBytes             int64
}

type TerminalAttachRequest struct {
	UserID     string
	Workspace  domain.Workspace
	TerminalID string
	ClientID   string
	Cols       int
	Rows       int
	SinceSeq   uint64
}

type TerminalRenameRequest struct {
	UserID     string
	Workspace  domain.Workspace
	TerminalID string
	Title      string
}

type TerminalAttachment struct {
	Session          TerminalSessionSummary
	History          []TerminalFrame
	HistoryTruncated bool
	Events           <-chan TerminalFrame
	Detach           func()
	Write            func([]byte) error
	Resize           func(int, int) error
	Terminate        func() error
}

type TerminalSessionSummary struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	WorkspaceID    string     `json:"workspace_id"`
	Title          string     `json:"title"`
	Shell          string     `json:"shell"`
	Status         string     `json:"status"`
	Cols           int        `json:"cols"`
	Rows           int        `json:"rows"`
	ExitCode       *int       `json:"exit_code,omitempty"`
	Error          string     `json:"error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastActiveAt   time.Time  `json:"last_active_at"`
	ExitedAt       *time.Time `json:"exited_at,omitempty"`
}

type TerminalFrame struct {
	Type       string                  `json:"type"`
	TerminalID string                  `json:"terminal_id,omitempty"`
	Session    *TerminalSessionSummary `json:"session,omitempty"`
	Seq        uint64                  `json:"seq,omitempty"`
	Data       string                  `json:"data,omitempty"`
	ExitCode   *int                    `json:"exit_code,omitempty"`
	Error      string                  `json:"error,omitempty"`
	Truncated  bool                    `json:"truncated,omitempty"`
}

type terminalManager struct {
	opts TerminalOptions

	mu         sync.Mutex
	sessions   map[string]*terminalSession
	createMu   sync.Mutex
	createKeys map[string]string

	reaperCancel context.CancelFunc
	reaperDone   chan struct{}
}

type terminalSession struct {
	mu sync.Mutex

	id             string
	organizationID string
	workspaceID    string
	userID         string
	workspacePath  string
	title          string
	shell          string
	status         string
	cols           int
	rows           int
	exitCode       *int
	errText        string
	createdAt      time.Time
	updatedAt      time.Time
	lastActiveAt   time.Time
	exitedAt       *time.Time

	cmd       *exec.Cmd
	ptmx      *os.File
	cancel    context.CancelFunc
	clients   map[chan TerminalFrame]struct{}
	history   *terminalHistory
	writeMu   sync.Mutex
	terminate sync.Once
	waitDone  chan struct{}
}

type terminalHistory struct {
	maxBytes        int64
	bytes           int64
	nextSeq         uint64
	truncatedBefore uint64
	chunks          []terminalHistoryChunk
}

type terminalHistoryChunk struct {
	Seq  uint64
	Data []byte
}

func newTerminalManager(opts TerminalOptions) *terminalManager {
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = defaultTerminalIdleTimeout
	}
	if opts.MaxSessionsPerWorkspace <= 0 {
		opts.MaxSessionsPerWorkspace = defaultTerminalMaxSessionsPerWorkspace
	}
	if opts.ReplayBytes <= 0 {
		opts.ReplayBytes = defaultTerminalReplayBytes
	}
	return &terminalManager{
		opts:       opts,
		sessions:   make(map[string]*terminalSession),
		createKeys: make(map[string]string),
	}
}

func (a *App) StartTerminalManager(ctx context.Context) {
	if a.terminals != nil {
		a.terminals.start(ctx)
	}
}

func (a *App) StopTerminalManager() {
	if a.terminals != nil {
		a.terminals.stop()
	}
}

func (a *App) ListTerminalSessions(_ context.Context, userID string, workspace domain.Workspace) []TerminalSessionSummary {
	return a.terminals.list(userID, workspace)
}

func (a *App) AttachTerminal(ctx context.Context, req TerminalAttachRequest) (TerminalAttachment, error) {
	return a.terminals.attach(ctx, req)
}

func (a *App) TerminateTerminal(_ context.Context, userID string, workspace domain.Workspace, terminalID string) error {
	return a.terminals.terminate(userID, workspace, terminalID)
}

func (a *App) RenameTerminal(_ context.Context, req TerminalRenameRequest) (TerminalSessionSummary, error) {
	return a.terminals.rename(req)
}

func (m *terminalManager) start(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reaperCancel != nil {
		return
	}
	reaperCtx, cancel := context.WithCancel(ctx)
	m.reaperCancel = cancel
	m.reaperDone = make(chan struct{})
	go m.reapLoop(reaperCtx, m.reaperDone)
}

func (m *terminalManager) stop() {
	m.mu.Lock()
	cancel := m.reaperCancel
	done := m.reaperDone
	m.reaperCancel = nil
	m.reaperDone = nil
	sessions := make([]*terminalSession, 0, len(m.sessions))
	for id, session := range m.sessions {
		sessions = append(sessions, session)
		delete(m.sessions, id)
	}
	m.createKeys = make(map[string]string)
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	for _, session := range sessions {
		session.kill()
	}
}

func (m *terminalManager) reapLoop(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pruneIdle(time.Now().UTC())
		}
	}
}

func (m *terminalManager) pruneIdle(now time.Time) {
	var stale []*terminalSession
	m.mu.Lock()
	for id, session := range m.sessions {
		if !session.idleExpired(now, m.opts.IdleTimeout) {
			continue
		}
		stale = append(stale, session)
		delete(m.sessions, id)
		m.deleteCreateKeysLocked(id)
	}
	m.mu.Unlock()
	for _, session := range stale {
		session.kill()
	}
}

func (m *terminalManager) list(userID string, workspace domain.Workspace) []TerminalSessionSummary {
	m.mu.Lock()
	sessions := make([]*terminalSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		if session.userID == userID && session.workspaceID == workspace.ID && session.organizationID == workspace.OrganizationID {
			sessions = append(sessions, session)
		}
	}
	m.mu.Unlock()

	summaries := make([]TerminalSessionSummary, 0, len(sessions))
	for _, session := range sessions {
		summaries = append(summaries, session.summary())
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].CreatedAt.Before(summaries[j].CreatedAt)
	})
	return summaries
}

func (m *terminalManager) attach(ctx context.Context, req TerminalAttachRequest) (TerminalAttachment, error) {
	if req.UserID == "" {
		return TerminalAttachment{}, ErrUnauthorized
	}
	if req.Workspace.ID == "" {
		return TerminalAttachment{}, ErrTerminalNotFound
	}
	if req.TerminalID == "" {
		if strings.TrimSpace(req.ClientID) != "" {
			m.createMu.Lock()
			defer m.createMu.Unlock()
		}
		return m.createAndAttach(ctx, req)
	}

	m.mu.Lock()
	session := m.sessions[req.TerminalID]
	m.mu.Unlock()
	if session == nil || !session.matches(req.UserID, req.Workspace) {
		return TerminalAttachment{}, ErrTerminalNotFound
	}
	if req.Cols > 0 && req.Rows > 0 {
		_ = session.resize(req.Cols, req.Rows)
	}
	return session.attach(req.SinceSeq), nil
}

func (m *terminalManager) createAndAttach(ctx context.Context, req TerminalAttachRequest) (TerminalAttachment, error) {
	if runtime.GOOS == "windows" {
		return TerminalAttachment{}, invalidInput("terminal PTY is not supported on windows")
	}
	if err := os.MkdirAll(req.Workspace.Path, 0o755); err != nil {
		return TerminalAttachment{}, err
	}

	createKey := terminalCreateKey(req)
	if createKey != "" {
		m.mu.Lock()
		existingID := m.createKeys[createKey]
		existing := m.sessions[existingID]
		m.mu.Unlock()
		if existing != nil && existing.matches(req.UserID, req.Workspace) {
			return existing.attach(req.SinceSeq), nil
		}
	}

	m.mu.Lock()
	if m.sessionCountLocked(req.UserID, req.Workspace) >= m.opts.MaxSessionsPerWorkspace {
		m.mu.Unlock()
		return TerminalAttachment{}, invalidInput(fmt.Sprintf("terminal limit reached (%d)", m.opts.MaxSessionsPerWorkspace))
	}
	m.mu.Unlock()

	cols, rows := normalizeTerminalSize(req.Cols, req.Rows)
	shell := resolveTerminalShell(m.opts.Shell)
	cmdCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(cmdCtx, shell)
	cmd.Dir = req.Workspace.Path
	cmd.Env = terminalEnv(os.Environ())

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		cancel()
		return TerminalAttachment{}, err
	}

	now := time.Now().UTC()
	session := &terminalSession{
		id:             id.New("term"),
		organizationID: req.Workspace.OrganizationID,
		workspaceID:    req.Workspace.ID,
		userID:         req.UserID,
		workspacePath:  req.Workspace.Path,
		title:          terminalTitle(shell),
		shell:          shell,
		status:         "running",
		cols:           cols,
		rows:           rows,
		createdAt:      now,
		updatedAt:      now,
		lastActiveAt:   now,
		cmd:            cmd,
		ptmx:           ptmx,
		cancel:         cancel,
		clients:        make(map[chan TerminalFrame]struct{}),
		history:        newTerminalHistory(m.opts.ReplayBytes),
		waitDone:       make(chan struct{}),
	}

	m.mu.Lock()
	m.sessions[session.id] = session
	if createKey != "" {
		m.createKeys[createKey] = session.id
	}
	m.mu.Unlock()

	go session.readLoop()
	slog.Info("terminal session started", "terminal_id", session.id, "workspace", req.Workspace.Path, "shell", shell)
	return session.attach(req.SinceSeq), nil
}

func (m *terminalManager) sessionCountLocked(userID string, workspace domain.Workspace) int {
	count := 0
	for _, session := range m.sessions {
		if session.userID == userID && session.workspaceID == workspace.ID && session.organizationID == workspace.OrganizationID {
			count++
		}
	}
	return count
}

func (m *terminalManager) terminate(userID string, workspace domain.Workspace, terminalID string) error {
	m.mu.Lock()
	session := m.sessions[terminalID]
	if session != nil && session.matches(userID, workspace) {
		delete(m.sessions, terminalID)
		m.deleteCreateKeysLocked(terminalID)
	} else {
		session = nil
	}
	m.mu.Unlock()
	if session == nil {
		return ErrTerminalNotFound
	}
	session.kill()
	return nil
}

func (m *terminalManager) rename(req TerminalRenameRequest) (TerminalSessionSummary, error) {
	if req.UserID == "" {
		return TerminalSessionSummary{}, ErrUnauthorized
	}
	if req.Workspace.ID == "" {
		return TerminalSessionSummary{}, ErrTerminalNotFound
	}
	title, err := normalizeTerminalTitle(req.Title)
	if err != nil {
		return TerminalSessionSummary{}, err
	}

	m.mu.Lock()
	session := m.sessions[req.TerminalID]
	m.mu.Unlock()
	if session == nil || !session.matches(req.UserID, req.Workspace) {
		return TerminalSessionSummary{}, ErrTerminalNotFound
	}
	return session.rename(title), nil
}

func (m *terminalManager) deleteCreateKeysLocked(terminalID string) {
	for key, value := range m.createKeys {
		if value == terminalID {
			delete(m.createKeys, key)
		}
	}
}

func (s *terminalSession) attach(sinceSeq uint64) TerminalAttachment {
	events := make(chan TerminalFrame, 256)
	s.mu.Lock()
	s.lastActiveAt = time.Now().UTC()
	s.updatedAt = s.lastActiveAt
	if s.status == "running" {
		s.clients[events] = struct{}{}
	} else {
		close(events)
	}
	history, truncated := s.history.framesSince(s.id, sinceSeq)
	summary := s.summaryLocked()
	s.mu.Unlock()

	detach := func() {
		s.mu.Lock()
		if _, ok := s.clients[events]; ok {
			delete(s.clients, events)
			close(events)
		}
		s.lastActiveAt = time.Now().UTC()
		s.updatedAt = s.lastActiveAt
		s.mu.Unlock()
	}

	return TerminalAttachment{
		Session:          summary,
		History:          history,
		HistoryTruncated: truncated,
		Events:           events,
		Detach:           detach,
		Write:            s.write,
		Resize:           s.resize,
		Terminate:        func() error { s.kill(); return nil },
	}
}

func (s *terminalSession) matches(userID string, workspace domain.Workspace) bool {
	return s.userID == userID && s.workspaceID == workspace.ID && s.organizationID == workspace.OrganizationID
}

func (s *terminalSession) summary() TerminalSessionSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.summaryLocked()
}

func (s *terminalSession) summaryLocked() TerminalSessionSummary {
	return TerminalSessionSummary{
		ID:             s.id,
		OrganizationID: s.organizationID,
		WorkspaceID:    s.workspaceID,
		Title:          s.title,
		Shell:          s.shell,
		Status:         s.status,
		Cols:           s.cols,
		Rows:           s.rows,
		ExitCode:       cloneIntPtr(s.exitCode),
		Error:          s.errText,
		CreatedAt:      s.createdAt,
		UpdatedAt:      s.updatedAt,
		LastActiveAt:   s.lastActiveAt,
		ExitedAt:       cloneTimePtr(s.exitedAt),
	}
}

func (s *terminalSession) rename(title string) TerminalSessionSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.title = title
	s.updatedAt = time.Now().UTC()
	return s.summaryLocked()
}

func (s *terminalSession) idleExpired(now time.Time, idleTimeout time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients) == 0 && now.Sub(s.lastActiveAt) >= idleTimeout
}

func (s *terminalSession) write(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if len(data) > maxTerminalInputBytes {
		return invalidInput("terminal input is too large")
	}
	s.mu.Lock()
	if s.status != "running" || s.ptmx == nil {
		s.mu.Unlock()
		return ErrTerminalExited
	}
	ptmx := s.ptmx
	s.lastActiveAt = time.Now().UTC()
	s.updatedAt = s.lastActiveAt
	s.mu.Unlock()

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := ptmx.Write(data)
	return err
}

func (s *terminalSession) resize(cols int, rows int) error {
	cols, rows = normalizeTerminalSize(cols, rows)
	s.mu.Lock()
	if s.status != "running" || s.ptmx == nil {
		s.mu.Unlock()
		return ErrTerminalExited
	}
	if s.cols == cols && s.rows == rows {
		s.mu.Unlock()
		return nil
	}
	ptmx := s.ptmx
	s.cols = cols
	s.rows = rows
	s.lastActiveAt = time.Now().UTC()
	s.updatedAt = s.lastActiveAt
	s.mu.Unlock()
	return pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (s *terminalSession) readLoop() {
	defer close(s.waitDone)
	buf := make([]byte, 32*1024)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			s.appendOutput(buf[:n])
		}
		if err != nil {
			s.finish(err)
			return
		}
	}
}

func (s *terminalSession) appendOutput(data []byte) {
	copied := append([]byte(nil), data...)
	s.mu.Lock()
	chunk := s.history.append(copied)
	frame := TerminalFrame{
		Type:       "output",
		TerminalID: s.id,
		Seq:        chunk.Seq,
		Data:       base64.StdEncoding.EncodeToString(chunk.Data),
	}
	s.updatedAt = time.Now().UTC()
	s.broadcastLocked(frame)
	s.mu.Unlock()
}

func (s *terminalSession) finish(readErr error) {
	waitErr := s.cmd.Wait()
	exitCode, errText := terminalExit(waitErr)
	if errText == "" && !terminalReadErrorIsNormal(readErr) {
		errText = readErr.Error()
	}
	now := time.Now().UTC()
	s.mu.Lock()
	if s.status != "exited" {
		s.status = "exited"
		s.exitCode = exitCode
		s.errText = errText
		s.exitedAt = &now
		s.updatedAt = now
		s.lastActiveAt = now
	}
	frame := TerminalFrame{
		Type:       "exit",
		TerminalID: s.id,
		ExitCode:   cloneIntPtr(s.exitCode),
		Error:      s.errText,
	}
	s.broadcastLocked(frame)
	for ch := range s.clients {
		close(ch)
		delete(s.clients, ch)
	}
	s.mu.Unlock()

	_ = s.ptmx.Close()
	slog.Info("terminal session exited", "terminal_id", s.id, "exit_code", exitCodeValue(exitCode), "error", errText)
}

func (s *terminalSession) broadcastLocked(frame TerminalFrame) {
	for ch := range s.clients {
		select {
		case ch <- frame:
		default:
			close(ch)
			delete(s.clients, ch)
		}
	}
}

func (s *terminalSession) kill() {
	s.terminate.Do(func() {
		s.mu.Lock()
		ptmx := s.ptmx
		cancel := s.cancel
		s.lastActiveAt = time.Now().UTC()
		s.updatedAt = s.lastActiveAt
		s.mu.Unlock()
		if ptmx != nil {
			_ = ptmx.Close()
		}
		if cancel != nil {
			cancel()
		}
		select {
		case <-s.waitDone:
		case <-time.After(3 * time.Second):
			if s.cmd != nil && s.cmd.Process != nil {
				_ = s.cmd.Process.Kill()
			}
		}
	})
}

func newTerminalHistory(maxBytes int64) *terminalHistory {
	return &terminalHistory{maxBytes: maxBytes}
}

func (h *terminalHistory) append(data []byte) terminalHistoryChunk {
	h.nextSeq++
	chunk := terminalHistoryChunk{Seq: h.nextSeq, Data: data}
	h.chunks = append(h.chunks, chunk)
	h.bytes += int64(len(data))
	for h.maxBytes > 0 && h.bytes > h.maxBytes && len(h.chunks) > 0 {
		removed := h.chunks[0]
		h.chunks = h.chunks[1:]
		h.bytes -= int64(len(removed.Data))
		h.truncatedBefore = removed.Seq
	}
	return chunk
}

func (h *terminalHistory) framesSince(terminalID string, sinceSeq uint64) ([]TerminalFrame, bool) {
	truncated := h.truncatedBefore > 0 && sinceSeq <= h.truncatedBefore
	frames := make([]TerminalFrame, 0, len(h.chunks))
	for _, chunk := range h.chunks {
		if chunk.Seq <= sinceSeq {
			continue
		}
		frames = append(frames, TerminalFrame{
			Type:       "output",
			TerminalID: terminalID,
			Seq:        chunk.Seq,
			Data:       base64.StdEncoding.EncodeToString(chunk.Data),
		})
	}
	return frames, truncated
}

func terminalCreateKey(req TerminalAttachRequest) string {
	clientID := strings.TrimSpace(req.ClientID)
	if clientID == "" {
		return ""
	}
	if len(clientID) > 128 {
		clientID = clientID[:128]
	}
	return req.UserID + ":" + req.Workspace.OrganizationID + ":" + req.Workspace.ID + ":" + clientID
}

func resolveTerminalShell(configured string) string {
	candidates := []string{strings.TrimSpace(configured), strings.TrimSpace(os.Getenv("SHELL")), "/bin/bash", "/bin/sh"}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if filepath.IsAbs(candidate) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
			continue
		}
		if found, err := exec.LookPath(candidate); err == nil {
			return found
		}
	}
	return "/bin/sh"
}

func terminalTitle(shell string) string {
	base := filepath.Base(shell)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "Terminal"
	}
	return base
}

func normalizeTerminalTitle(value string) (string, error) {
	title := strings.TrimSpace(value)
	if title == "" {
		return "", invalidInput("terminal title is required")
	}
	if len([]rune(title)) > maxTerminalTitleRunes {
		return "", invalidInput(fmt.Sprintf("terminal title must be at most %d characters", maxTerminalTitleRunes))
	}
	for _, char := range title {
		if char < 0x20 || char == 0x7f {
			return "", invalidInput("terminal title contains invalid control characters")
		}
	}
	return title, nil
}

func terminalEnv(base []string) []string {
	values := make(map[string]string, len(base)+3)
	order := make([]string, 0, len(base)+3)
	for _, item := range base {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			continue
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = value
	}
	set := func(key, value string) {
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = value
	}
	set("TERM", "xterm-256color")
	set("COLORTERM", "truecolor")
	set("TERM_PROGRAM", "AgentX")
	env := make([]string, 0, len(order))
	for _, key := range order {
		env = append(env, key+"="+values[key])
	}
	return env
}

func normalizeTerminalSize(cols int, rows int) (int, int) {
	if cols < 2 {
		cols = defaultTerminalCols
	}
	if rows < 2 {
		rows = defaultTerminalRows
	}
	if cols > 500 {
		cols = 500
	}
	if rows > 300 {
		rows = 300
	}
	return cols, rows
}

func terminalExit(err error) (*int, string) {
	if err == nil {
		code := 0
		return &code, ""
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		if code < 0 {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				return nil, "terminated by signal " + status.Signal().String()
			}
		}
		return &code, fmt.Sprintf("exited with status %d", code)
	}
	return nil, err.Error()
}

func terminalReadErrorIsNormal(err error) bool {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "input/output error") || strings.Contains(text, "file already closed")
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func exitCodeValue(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
