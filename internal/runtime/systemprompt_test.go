package runtime

import (
	"os"
	"strings"
	"testing"
)

func TestSystemPromptModes(t *testing.T) {
	req := StartSessionRequest{SystemPrompt: " be terse ", SystemPromptMode: "REPLACE"}
	if !req.ReplacesSystemPrompt() || req.AppendedSystemPrompt() != "" {
		t.Fatalf("replace mode: replaces=%v appended=%q", req.ReplacesSystemPrompt(), req.AppendedSystemPrompt())
	}
	req.SystemPromptMode = ""
	if req.ReplacesSystemPrompt() || req.AppendedSystemPrompt() != "be terse" {
		t.Fatalf("default mode: replaces=%v appended=%q", req.ReplacesSystemPrompt(), req.AppendedSystemPrompt())
	}
	req.SystemPrompt = "  "
	req.SystemPromptMode = SystemPromptModeReplace
	if req.ReplacesSystemPrompt() || req.AppendedSystemPrompt() != "" {
		t.Fatal("an empty prompt never replaces anything")
	}
	if NormalizeSystemPromptMode("other") != SystemPromptModeAppend {
		t.Fatal("unknown mode should normalise to append")
	}
}

func TestSystemPromptFileIsContentAddressed(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	first, err := SystemPromptFile(" hello ")
	if err != nil {
		t.Fatal(err)
	}
	again, err := SystemPromptFile("hello")
	if err != nil {
		t.Fatal(err)
	}
	other, err := SystemPromptFile("hello there")
	if err != nil {
		t.Fatal(err)
	}
	if first != again || first == other {
		t.Fatalf("paths: %s %s %s", first, again, other)
	}
	data, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "hello" {
		t.Fatalf("content = %q", data)
	}
}
