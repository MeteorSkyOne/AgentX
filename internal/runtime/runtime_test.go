package runtime

import (
	"strings"
	"testing"
)

func TestRenderedPromptIncludesAttachmentSizeWithoutKindOrContentType(t *testing.T) {
	prompt := Input{
		Prompt: "inspect",
		Attachments: []Attachment{{
			ID:        "att_size_only",
			Filename:  "data",
			SizeBytes: 123,
			LocalPath: "/tmp/data",
		}},
	}.RenderedPrompt()

	if !strings.Contains(prompt, "data (123 bytes): /tmp/data") {
		t.Fatalf("prompt = %q, want size-only attachment details", prompt)
	}
}
