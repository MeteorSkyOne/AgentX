package pi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

// record is one JSONL record read from or written to pi.
type record = map[string]any

// eventBuffer is how many session events the client keeps while no session is
// consuming them. When it fills up the oldest event is dropped so pi's reader
// never stalls and the newest record (typically agent_settled) survives.
const eventBuffer = 1024

type rpcError struct {
	Command string
	Message string
}

func (e *rpcError) Error() string {
	if e.Command == "" {
		return e.Message
	}
	return fmt.Sprintf("pi %s: %s", e.Command, e.Message)
}

// rpcClient correlates pi RPC commands with their responses by id and hands
// every other stdout record (session events, extension UI requests) to the
// Events channel.
type rpcClient struct {
	process *procpool.ManagedProcess
	nextID  atomic.Int64

	mu            sync.Mutex
	pending       map[string]chan record
	eventCh       chan record
	closed        bool
	sessionIDSeen string
	modelSeen     string
}

func newRPCClient(process *procpool.ManagedProcess) *rpcClient {
	c := &rpcClient{
		process: process,
		pending: make(map[string]chan record),
		eventCh: make(chan record, eventBuffer),
	}
	go c.readLoop()
	return c
}

// Call sends a command and waits for its response. It returns the response's
// data field (nil when the command carries none).
func (c *rpcClient) Call(ctx context.Context, cmd record) (any, error) {
	id := "agentx-" + strconv.FormatInt(c.nextID.Add(1), 10)
	msg := make(record, len(cmd)+1)
	for k, v := range cmd {
		msg[k] = v
	}
	msg["id"] = id
	command, _ := cmd["type"].(string)

	ch := make(chan record, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, procpool.ErrProcessDead
	}
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.process.WriteJSON(msg); err != nil {
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
		if success, _ := resp["success"].(bool); !success {
			message := stringVal(resp, "error")
			if message == "" {
				message = "command failed"
			}
			return nil, &rpcError{Command: command, Message: message}
		}
		if command == "get_state" {
			if data, ok := resp["data"].(record); ok {
				c.mu.Lock()
				if sid := stringVal(data, "sessionId"); sid != "" {
					c.sessionIDSeen = sid
				}
				if model := stringVal(mapVal(data, "model"), "id"); model != "" {
					c.modelSeen = model
				}
				c.mu.Unlock()
			}
		}
		return resp["data"], nil
	}
}

// CallMap is Call for commands whose data is a JSON object.
func (c *rpcClient) CallMap(ctx context.Context, cmd record) (record, error) {
	data, err := c.Call(ctx, cmd)
	if err != nil {
		return nil, err
	}
	m, _ := data.(record)
	return m, nil
}

// Send writes a record that expects no response (extension_ui_response).
func (c *rpcClient) Send(msg record) error {
	return c.process.WriteJSON(msg)
}

func (c *rpcClient) Events() <-chan record {
	return c.eventCh
}

// lastSessionID returns the pi session id most recently reported by get_state.
func (c *rpcClient) lastSessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionIDSeen
}

// lastModel returns the model id most recently reported by get_state.
func (c *rpcClient) lastModel() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.modelSeen
}

// drain discards events buffered from a previous turn on this pooled process.
func (c *rpcClient) drain() {
	for {
		select {
		case <-c.eventCh:
		default:
			return
		}
	}
}

func (c *rpcClient) readLoop() {
	defer func() {
		c.mu.Lock()
		c.closed = true
		for _, ch := range c.pending {
			close(ch)
		}
		c.pending = make(map[string]chan record)
		close(c.eventCh)
		c.mu.Unlock()
	}()

	for line := range c.process.StdoutLines() {
		var rec record
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		c.dispatch(rec)
	}
}

func (c *rpcClient) dispatch(rec record) {
	if stringVal(rec, "type") == "response" {
		id := stringVal(rec, "id")
		c.mu.Lock()
		ch, ok := c.pending[id]
		c.mu.Unlock()
		if ok {
			select {
			case ch <- rec:
			default:
			}
		}
		return
	}
	for {
		select {
		case c.eventCh <- rec:
			return
		default:
		}
		// Full: drop the oldest event and retry.
		select {
		case <-c.eventCh:
		default:
		}
	}
}

var errNoActiveTurn = errors.New("no active turn")

func stringVal(m record, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func mapVal(m record, key string) record {
	if m == nil {
		return nil
	}
	v, _ := m[key].(record)
	return v
}

func boolVal(m record, key string) bool {
	if m == nil {
		return false
	}
	v, _ := m[key].(bool)
	return v
}

func sliceVal(m record, key string) []any {
	if m == nil {
		return nil
	}
	v, _ := m[key].([]any)
	return v
}

func int64Ptr(m record, key string) *int64 {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case int:
		i := int64(n)
		return &i
	case int64:
		return &n
	case float64:
		i := int64(n)
		return &i
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return nil
		}
		return &i
	}
	return nil
}

func float64Ptr(m record, key string) *float64 {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case int:
		f := float64(n)
		return &f
	case int64:
		f := float64(n)
		return &f
	case float64:
		return &n
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return nil
		}
		return &f
	}
	return nil
}
