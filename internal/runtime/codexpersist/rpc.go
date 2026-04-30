package codexpersist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/meteorsky/agentx/internal/runtime/procpool"
)

type jsonRPCMessage struct {
	JSONRPC string         `json:"jsonrpc"`
	Method  string         `json:"method,omitempty"`
	ID      any            `json:"id,omitempty"`
	Params  any            `json:"params,omitempty"`
	Result  map[string]any `json:"result,omitempty"`
	Error   *jsonRPCError  `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

type pendingRequest struct {
	ch chan jsonRPCMessage
}

type rpcClient struct {
	process *procpool.ManagedProcess
	nextID  atomic.Int64

	mu       sync.Mutex
	pending  map[int64]*pendingRequest
	notifyCh chan jsonRPCMessage
	closed   bool
}

func newRPCClient(process *procpool.ManagedProcess) *rpcClient {
	c := &rpcClient{
		process:  process,
		pending:  make(map[int64]*pendingRequest),
		notifyCh: make(chan jsonRPCMessage, 64),
	}
	go c.readLoop()
	return c
}

func (c *rpcClient) Call(ctx context.Context, method string, params any) (map[string]any, error) {
	id := c.nextID.Add(1)
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"id":      id,
		"params":  params,
	}

	req := &pendingRequest{ch: make(chan jsonRPCMessage, 1)}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("rpc client closed")
	}
	c.pending[id] = req
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
	case resp, ok := <-req.ch:
		if !ok {
			return nil, procpool.ErrProcessDead
		}
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
}

func (c *rpcClient) Notify(method string, params any) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	return c.process.WriteJSON(msg)
}

func (c *rpcClient) Notifications() <-chan jsonRPCMessage {
	return c.notifyCh
}

func (c *rpcClient) RespondToRequest(id any, result any) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	return c.process.WriteJSON(msg)
}

func (c *rpcClient) readLoop() {
	defer func() {
		c.mu.Lock()
		c.closed = true
		for _, req := range c.pending {
			close(req.ch)
		}
		c.pending = make(map[int64]*pendingRequest)
		close(c.notifyCh)
		c.mu.Unlock()
	}()

	for line := range c.process.StdoutLines() {
		var msg jsonRPCMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		c.dispatch(msg)
	}
}

func (c *rpcClient) dispatch(msg jsonRPCMessage) {
	if msg.ID != nil && msg.Method == "" {
		id := jsonNumberToInt64(msg.ID)
		c.mu.Lock()
		req, ok := c.pending[id]
		c.mu.Unlock()
		if ok {
			select {
			case req.ch <- msg:
			default:
			}
		}
		return
	}

	if msg.Method != "" && msg.ID != nil {
		select {
		case c.notifyCh <- msg:
		default:
		}
		return
	}

	if msg.Method != "" {
		select {
		case c.notifyCh <- msg:
		default:
		}
	}
}

func jsonNumberToInt64(v any) int64 {
	switch typed := v.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case json.Number:
		n, _ := typed.Int64()
		return n
	default:
		return 0
	}
}
