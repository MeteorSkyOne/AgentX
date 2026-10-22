package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// System prompt modes: how an agent's SystemPrompt combines with the
// runtime's built-in system prompt.
const (
	// SystemPromptModeAppend adds the agent prompt after the runtime's default.
	SystemPromptModeAppend = "append"
	// SystemPromptModeReplace uses the agent prompt instead of the runtime's default.
	SystemPromptModeReplace = "replace"
)

// NormalizeSystemPromptMode maps any input onto a known mode, defaulting to append.
func NormalizeSystemPromptMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), SystemPromptModeReplace) {
		return SystemPromptModeReplace
	}
	return SystemPromptModeAppend
}

// ReplacesSystemPrompt reports whether the request asks for the runtime's
// default system prompt to be replaced by the agent prompt.
func (r StartSessionRequest) ReplacesSystemPrompt() bool {
	return strings.TrimSpace(r.SystemPrompt) != "" && NormalizeSystemPromptMode(r.SystemPromptMode) == SystemPromptModeReplace
}

// AppendedSystemPrompt returns the agent prompt when it should be appended to
// the runtime's default, and "" when it is empty or replaces the default.
func (r StartSessionRequest) AppendedSystemPrompt() string {
	if r.ReplacesSystemPrompt() {
		return ""
	}
	return strings.TrimSpace(r.SystemPrompt)
}

// SystemPromptFile writes prompt to a content-addressed file for CLIs that
// only take a replacement system prompt from a path, and returns that path.
func SystemPromptFile(prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	sum := sha256.Sum256([]byte(prompt))
	dir := filepath.Join(os.TempDir(), "agentx-system-prompts")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, hex.EncodeToString(sum[:8])+".md")
	if existing, err := os.ReadFile(path); err == nil && string(existing) == prompt+"\n" {
		return path, nil
	}
	if err := os.WriteFile(path, []byte(prompt+"\n"), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
