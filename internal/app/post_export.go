package app

import (
	"context"
	"strings"

	"github.com/meteorsky/agentx/internal/transcript"
)

// ExportThread loads a forum post with its full message history.
func (a *App) ExportThread(ctx context.Context, threadID string) (transcript.Post, error) {
	return transcript.Load(ctx, a.store, threadID)
}

// MCPSetup describes how a local AI client launches the read-only post MCP
// server against this instance's database.
type MCPSetup struct {
	Command       string   `json:"command"`
	Args          []string `json:"args"`
	ClaudeCommand string   `json:"claude_command"`
	CodexCommand  string   `json:"codex_command"`
	PiCommand     string   `json:"pi_command"`
	Available     bool     `json:"available"`
}

func (a *App) MCPSetup() MCPSetup {
	command := strings.TrimSpace(a.opts.MCPCommand)
	if command == "" {
		command = "agentx"
	}
	args := []string{"mcp"}
	if path := strings.TrimSpace(a.opts.SQLitePath); path != "" {
		args = append(args, "--db", path)
	}
	launch := shellQuote(command)
	for _, arg := range args {
		launch += " " + shellQuote(arg)
	}
	return MCPSetup{
		Command:       command,
		Args:          args,
		ClaudeCommand: "claude mcp add --scope user agentx -- " + launch,
		CodexCommand:  "codex mcp add agentx -- " + launch,
		PiCommand:     "pi mcp add --exposure direct agentx -- " + launch,
		Available:     strings.TrimSpace(a.opts.MCPCommand) != "",
	}
}

func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:=@+,", r))
	}) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
