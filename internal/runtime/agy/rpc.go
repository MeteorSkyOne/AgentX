package agy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/meteorsky/agentx/internal/runtime"
	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

type record = map[string]any

type rpcError struct {
	Method  string
	Code    int
	Message string
	Data    any
}

func (e *rpcError) Error() string {
	if e.Method == "" {
		return fmt.Sprintf("acp error (%d): %s", e.Code, e.Message)
	}
	return fmt.Sprintf("acp %s (%d): %s", e.Method, e.Code, e.Message)
}

func isMissingSessionError(err error) bool {
	var rerr *rpcError
	if !errors.As(err, &rerr) {
		return false
	}
	lower := strings.ToLower(rerr.Message)
	for _, needle := range []string{
		"failed to read session",
		"session not found",
		"no such session",
		"unknown session",
		"does not exist",
		"cannot find session",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// rpcMessage represents either a server notification (IDPresent == false) or a
// server-to-client request such as `session/request_permission` (IDPresent == true).
type rpcMessage struct {
	ID        any
	IDPresent bool
	Method    string
	Params    record
}

type rpcResponse struct {
	Result any
	Err    *rpcError
}

type rpcClient struct {
	process *procpool.ManagedProcess
	nextID  atomic.Int64

	mu      sync.Mutex
	pending map[string]chan rpcResponse
	closed  bool

	initialized      bool
	activeSessionID  string
	configOptions    []configOption
	currentModelID   string
	currentModeID    string
	lastContextUsage *runtime.ContextUsage

	queueMu     sync.Mutex
	queueCond   *sync.Cond
	queue       []rpcMessage
	queueClosed bool
	msgCh       chan rpcMessage
}

func newRPCClient(process *procpool.ManagedProcess) *rpcClient {
	c := &rpcClient{
		process: process,
		pending: make(map[string]chan rpcResponse),
		msgCh:   make(chan rpcMessage, 64),
	}
	c.queueCond = sync.NewCond(&c.queueMu)
	go c.pumpLoop()
	go c.readLoop()
	return c
}

func (c *rpcClient) Call(ctx context.Context, method string, params any) (any, error) {
	id := c.nextID.Add(1)
	key := strconv.FormatInt(id, 10)

	ch := make(chan rpcResponse, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, procpool.ErrProcessDead
	}
	c.pending[key] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, key)
		c.mu.Unlock()
	}()

	req := record{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	if err := c.process.WriteJSON(req); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.process.Done():
		return nil, procpool.ErrProcessDead
	case resp, ok := <-ch:
		if !ok {
			return nil, procpool.ErrProcessDead
		}
		if resp.Err != nil {
			errCopy := *resp.Err
			errCopy.Method = method
			return nil, &errCopy
		}
		return resp.Result, nil
	}
}

func (c *rpcClient) CallMap(ctx context.Context, method string, params any) (record, error) {
	res, err := c.Call(ctx, method, params)
	if err != nil {
		return nil, err
	}
	m, _ := res.(record)
	return m, nil
}

// Notify sends a JSON-RPC 2.0 notification (no id, no response expected).
func (c *rpcClient) Notify(method string, params any) error {
	msg := record{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	return c.process.WriteJSON(msg)
}

// Respond replies to a server-to-client request, preserving the exact JSON id
// type (string or number) sent by the server.
func (c *rpcClient) Respond(id any, result any) error {
	return c.process.WriteJSON(record{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func (c *rpcClient) RespondError(id any, code int, message string) error {
	return c.process.WriteJSON(record{
		"jsonrpc": "2.0",
		"id":      id,
		"error": record{
			"code":    code,
			"message": message,
		},
	})
}

func (c *rpcClient) Messages() <-chan rpcMessage {
	return c.msgCh
}

// drainAndCacheUpdates discards queued messages from a prior turn while
// preserving any session state updates they carried.
func (c *rpcClient) drainAndCacheUpdates() {
	c.queueMu.Lock()
	queued := c.queue
	c.queue = nil
	c.queueMu.Unlock()
	for _, msg := range queued {
		c.observeMessage(msg)
	}
	for {
		select {
		case msg, ok := <-c.msgCh:
			if !ok {
				return
			}
			c.observeMessage(msg)
		default:
			return
		}
	}
}

func (c *rpcClient) pumpLoop() {
	defer close(c.msgCh)
	for {
		c.queueMu.Lock()
		for len(c.queue) == 0 && !c.queueClosed {
			c.queueCond.Wait()
		}
		if len(c.queue) == 0 && c.queueClosed {
			c.queueMu.Unlock()
			return
		}
		batch := c.queue
		c.queue = nil
		c.queueMu.Unlock()

		for _, msg := range batch {
			c.msgCh <- msg
		}
	}
}

func (c *rpcClient) enqueue(msg rpcMessage) {
	c.observeMessage(msg)
	c.queueMu.Lock()
	if !c.queueClosed {
		c.queue = append(c.queue, msg)
		c.queueCond.Signal()
	}
	c.queueMu.Unlock()
}

func (c *rpcClient) readLoop() {
	defer func() {
		c.mu.Lock()
		c.closed = true
		for _, ch := range c.pending {
			close(ch)
		}
		c.pending = make(map[string]chan rpcResponse)
		c.mu.Unlock()

		c.queueMu.Lock()
		c.queueClosed = true
		c.queueCond.Broadcast()
		c.queueMu.Unlock()
	}()

	for line := range c.process.StdoutLines() {
		var raw record
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		method := stringVal(raw, "method")
		rawID, hasID := raw["id"]
		if hasID && rawID == nil {
			hasID = false
		}

		if method != "" {
			params, _ := raw["params"].(record)
			c.enqueue(rpcMessage{
				ID:        rawID,
				IDPresent: hasID,
				Method:    method,
				Params:    params,
			})
			continue
		}

		if !hasID {
			continue
		}
		key := idKey(rawID)
		var resp rpcResponse
		if errObj, ok := raw["error"].(record); ok && errObj != nil {
			resp.Err = &rpcError{
				Code:    int(int64Val(errObj, "code")),
				Message: stringVal(errObj, "message"),
				Data:    errObj["data"],
			}
			if resp.Err.Message == "" {
				resp.Err.Message = "rpc request failed"
			}
		} else {
			resp.Result = raw["result"]
		}

		c.mu.Lock()
		ch, ok := c.pending[key]
		c.mu.Unlock()
		if ok {
			select {
			case ch <- resp:
			default:
			}
		}
	}
}

func idKey(v any) string {
	switch id := v.(type) {
	case string:
		return id
	case float64:
		return strconv.FormatInt(int64(id), 10)
	case int64:
		return strconv.FormatInt(id, 10)
	case int:
		return strconv.Itoa(id)
	case json.Number:
		return id.String()
	default:
		return fmt.Sprint(v)
	}
}

func (c *rpcClient) isInitialized() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.initialized
}

func (c *rpcClient) setInitialized() {
	c.mu.Lock()
	c.initialized = true
	c.mu.Unlock()
}

func (c *rpcClient) sessionState() (sessionID string, modelID string, modeID string, opts []configOption) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cloned := make([]configOption, len(c.configOptions))
	copy(cloned, c.configOptions)
	return c.activeSessionID, c.currentModelID, c.currentModeID, cloned
}

func (c *rpcClient) setActiveSession(sessionID string, result record) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activeSessionID != sessionID {
		c.lastContextUsage = nil
	}
	c.activeSessionID = sessionID
	c.applySessionPayloadLocked(result)
}

func (c *rpcClient) clearActiveSession() {
	c.mu.Lock()
	c.activeSessionID = ""
	c.configOptions = nil
	c.currentModelID = ""
	c.currentModeID = ""
	c.lastContextUsage = nil
	c.mu.Unlock()
}

func (c *rpcClient) updateConfigFromResult(result record) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.applySessionPayloadLocked(result)
}

func (c *rpcClient) applySessionPayloadLocked(payload record) {
	if payload == nil {
		return
	}
	if models := mapVal(payload, "models"); models != nil {
		if cur := stringVal(models, "currentModelId"); cur != "" {
			c.currentModelID = cur
		}
	}
	if modes := mapVal(payload, "modes"); modes != nil {
		if cur := stringVal(modes, "currentModeId"); cur != "" {
			c.currentModeID = cur
		}
	}
	if rawOpts := sliceVal(payload, "configOptions"); len(rawOpts) > 0 {
		opts := parseConfigOptions(rawOpts)
		c.configOptions = opts
		if m, ok := optionByID(opts, "model"); ok && m.CurrentValue != "" {
			c.currentModelID = m.CurrentValue
		}
		if mode, ok := optionByID(opts, "mode"); ok && mode.CurrentValue != "" {
			c.currentModeID = mode.CurrentValue
		}
	}
}

func (c *rpcClient) observeMessage(msg rpcMessage) {
	if msg.Method != "session/update" || msg.Params == nil {
		return
	}
	sid := stringVal(msg.Params, "sessionId")
	update := mapVal(msg.Params, "update")
	if update == nil {
		return
	}
	kind := stringVal(update, "sessionUpdate")
	c.mu.Lock()
	defer c.mu.Unlock()
	if sid != "" && c.activeSessionID != "" && sid != c.activeSessionID {
		return
	}
	switch kind {
	case "config_option_update":
		c.applySessionPayloadLocked(update)
	case "current_mode_update":
		if cur := stringVal(update, "currentModeId"); cur != "" {
			c.currentModeID = cur
		}
	case "usage_update":
		if usage := parseContextUsageUpdate(update); usage != nil {
			c.lastContextUsage = usage
		}
	}
}

func (c *rpcClient) contextUsage() *runtime.ContextUsage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneContextUsage(c.lastContextUsage)
}
