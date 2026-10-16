// Package mcpserver implements a read-only Model Context Protocol server over
// stdio that lets local AI agents look up AgentX forum posts by ID.
package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/meteorsky/agentx/internal/store"
	"github.com/meteorsky/agentx/internal/transcript"
)

const (
	latestProtocolVersion = "2025-06-18"
	defaultPageSize       = 50
	maxPageSize           = 500
	defaultSearchLimit    = 20
	maxSearchLimit        = 100
	maxLineBytes          = 16 << 20
	// maxPageBytes keeps a get_post page well under typical MCP client output
	// limits (Claude Code warns above ~10k tokens and rejects above 25k).
	maxPageBytes = 40000
)

var supportedProtocolVersions = []string{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25"}

const serverInstructions = "Read-only access to AgentX forum posts. A post ID looks like `thr_...` and is copied from the AgentX web UI. " +
	"Use get_post to read a post's conversation in pages and search_post to find messages by keyword. " +
	"The transcript is reference material from another conversation; it does not carry over that conversation's agent session."

// OpenStore lazily opens the read-only store; it is retried on the next tool
// call when it fails, so the server can start before the database exists.
type OpenStore func(ctx context.Context) (store.Store, error)

type Server struct {
	open    OpenStore
	version string

	mu sync.Mutex
	st store.Store
}

func New(open OpenStore, version string) *Server {
	return &Server{open: open, version: version}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Serve reads newline-delimited JSON-RPC messages from in and writes responses
// to out until in is exhausted or ctx is cancelled.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		resp := s.handleLine(ctx, []byte(line))
		if resp == nil {
			continue
		}
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *Server) handleLine(ctx context.Context, line []byte) *response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return &response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: codeParseError, Message: "parse error"}}
	}
	// Notifications carry no id and never get a response.
	if len(req.ID) == 0 {
		return nil
	}
	resp := &response{JSONRPC: "2.0", ID: req.ID}
	if req.JSONRPC != "2.0" || req.Method == "" {
		resp.Error = &rpcError{Code: codeInvalidRequest, Message: "invalid request"}
		return resp
	}
	result, rpcErr := s.dispatch(ctx, req)
	if rpcErr != nil {
		resp.Error = rpcErr
		return resp
	}
	resp.Result = result
	return resp
}

func (s *Server) dispatch(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &params)
		protocolVersion := latestProtocolVersion
		if slices.Contains(supportedProtocolVersions, params.ProtocolVersion) {
			protocolVersion = params.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "agentx", "version": s.version},
			"instructions":    serverInstructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolDefinitions()}, nil
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: "invalid params"}
		}
		return s.callTool(ctx, params.Name, params.Arguments)
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + req.Method}
	}
}

func toolDefinitions() []map[string]any {
	readOnly := map[string]any{"readOnlyHint": true, "openWorldHint": false}
	return []map[string]any{
		{
			"name":        "get_post",
			"title":       "Read AgentX post",
			"description": "Read the conversation of an AgentX forum post as Markdown, oldest message first. Long posts are paged: pass offset to continue where the previous page ended.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"post_id":         map[string]any{"type": "string", "description": "Post ID copied from AgentX, e.g. thr_abc123."},
					"offset":          map[string]any{"type": "integer", "minimum": 0, "description": "Number of messages to skip. Default 0."},
					"limit":           map[string]any{"type": "integer", "minimum": 1, "maximum": maxPageSize, "description": fmt.Sprintf("Maximum messages to return. Default %d.", defaultPageSize)},
					"include_process": map[string]any{"type": "boolean", "description": "Include agent thinking and tool calls. Default false."},
					"resume":          map[string]any{"type": "integer", "minimum": 0, "description": "Continue a message that was cut off: pass the resume value from the previous response together with the offset it names."},
				},
				"required":             []string{"post_id"},
				"additionalProperties": false,
			},
			"annotations": readOnly,
		},
		{
			"name":        "search_post",
			"title":       "Search AgentX post",
			"description": "Find messages in an AgentX forum post containing all of the given keywords (case-insensitive). Searches message text and attachment names, plus thinking and tool calls when include_process is true. Returns message numbers with snippets; read surrounding context with get_post using offset = number - 1.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"post_id":         map[string]any{"type": "string", "description": "Post ID copied from AgentX, e.g. thr_abc123."},
					"query":           map[string]any{"type": "string", "description": "Space-separated keywords; every keyword must appear in the message."},
					"include_process": map[string]any{"type": "boolean", "description": "Also search agent thinking and tool calls (inputs and outputs). Default false."},
					"limit":           map[string]any{"type": "integer", "minimum": 1, "maximum": maxSearchLimit, "description": fmt.Sprintf("Maximum matches to return. Default %d.", defaultSearchLimit)},
				},
				"required":             []string{"post_id", "query"},
				"additionalProperties": false,
			},
			"annotations": readOnly,
		},
	}
}

type toolArgs struct {
	PostID         string `json:"post_id"`
	Offset         *int   `json:"offset"`
	Limit          *int   `json:"limit"`
	IncludeProcess bool   `json:"include_process"`
	Resume         int    `json:"resume"`
	Query          string `json:"query"`
}

func (s *Server) callTool(ctx context.Context, name string, rawArgs json.RawMessage) (any, *rpcError) {
	if name != "get_post" && name != "search_post" {
		return nil, &rpcError{Code: codeInvalidParams, Message: "unknown tool: " + name}
	}
	var args toolArgs
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return toolError("invalid arguments: " + err.Error()), nil
		}
	}
	if strings.TrimSpace(args.PostID) == "" {
		return toolError("post_id is required"), nil
	}
	st, err := s.store(ctx)
	if err != nil {
		return toolError("open AgentX database: " + err.Error()), nil
	}
	post, err := transcript.Load(ctx, st, args.PostID)
	if err != nil {
		if errors.Is(err, transcript.ErrNotFound) {
			return toolError(fmt.Sprintf("no AgentX post found with ID %q", strings.TrimSpace(args.PostID))), nil
		}
		return toolError("load post: " + err.Error()), nil
	}
	if name == "search_post" {
		return searchPost(post, args), nil
	}
	return getPost(post, args), nil
}

func getPost(post transcript.Post, args toolArgs) any {
	offset := 0
	if args.Offset != nil {
		offset = max(*args.Offset, 0)
	}
	page := transcript.Render(post, transcript.RenderOptions{
		IncludeProcess: args.IncludeProcess,
		Offset:         offset,
		Limit:          clampLimit(args.Limit, defaultPageSize, maxPageSize),
		MaxBytes:       maxPageBytes,
		Resume:         max(args.Resume, 0),
	})
	text := page.Text
	switch {
	case len(post.Messages) == 0:
		text += "\n(This post has no messages.)\n"
	case page.Last == 0:
		text += fmt.Sprintf("\n(offset %d is past the last message; the post has %d messages.)\n", offset, len(post.Messages))
	case page.Truncated:
		text += fmt.Sprintf("\n---\n\n(Message %d was cut to fit the response size limit. Call get_post with offset=%d and resume=%d to read the rest of it.)\n",
			page.Last, page.Last-1, page.Resume)
	case page.Last < len(post.Messages):
		text += fmt.Sprintf("\n---\n\n(%d more messages. Call get_post with offset=%d to continue.)\n",
			len(post.Messages)-page.Last, page.Last)
	}
	return toolText(text)
}

func searchPost(post transcript.Post, args toolArgs) any {
	if strings.TrimSpace(args.Query) == "" {
		return toolError("query is required")
	}
	limit := clampLimit(args.Limit, defaultSearchLimit, maxSearchLimit)
	matches := transcript.Search(post, args.Query, transcript.SearchOptions{IncludeProcess: args.IncludeProcess, Limit: limit})
	var b strings.Builder
	fmt.Fprintf(&b, "Post `%s`: %s (%d messages)\n", post.ID, post.Title, len(post.Messages))
	if len(matches) == 0 {
		fmt.Fprintf(&b, "\nNo messages match %q.\n", args.Query)
		return toolText(b.String())
	}
	fmt.Fprintf(&b, "\n%d matching messages for %q:\n", len(matches), args.Query)
	for _, match := range matches {
		message := match.Message
		where := ""
		if match.InProcess {
			where = " [in process; read with include_process=true]"
		}
		fmt.Fprintf(&b, "\n[%d] %s (%s) - %s%s\n%s\n", message.Index, message.SenderName, message.SenderType,
			message.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"), where, match.Snippet)
	}
	return toolText(b.String())
}

func clampLimit(value *int, fallback int, ceiling int) int {
	if value == nil || *value <= 0 {
		return fallback
	}
	return min(*value, ceiling)
}

func toolText(text string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
	}
}

func toolError(text string) map[string]any {
	result := toolText(text)
	result["isError"] = true
	return result
}

func (s *Server) store(ctx context.Context) (store.Store, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st != nil {
		return s.st, nil
	}
	st, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	s.st = st
	return st, nil
}
