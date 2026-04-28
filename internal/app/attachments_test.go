package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
)

func TestPrepareMessageAttachmentsRejectsEmptyAttachment(t *testing.T) {
	application := &App{opts: Options{DataDir: t.TempDir()}}
	_, err := application.prepareMessageAttachments(domain.Message{
		ID:               "msg_empty_attachment",
		OrganizationID:   "org_empty_attachment",
		ConversationType: domain.ConversationChannel,
		ConversationID:   "chn_empty_attachment",
		CreatedAt:        time.Now().UTC(),
	}, []AttachmentUpload{{
		Filename:    "empty.txt",
		ContentType: "text/plain",
		Data:        nil,
	}})
	if !errors.Is(err, ErrInvalidInput) || InvalidInputMessage(err) != "empty attachment" {
		t.Fatalf("empty attachment error = %v, want empty attachment invalid input", err)
	}
}

func TestSanitizeAttachmentFilenameTruncatesBeforeExtension(t *testing.T) {
	name := sanitizeAttachmentFilename(strings.Repeat("a", 220) + ".tsx")
	if !strings.HasSuffix(name, ".tsx") {
		t.Fatalf("filename = %q, want .tsx suffix", name)
	}
	if strings.HasSuffix(strings.TrimSuffix(name, ".tsx"), ".ts") {
		t.Fatalf("filename = %q, want no partial duplicate extension", name)
	}
	if got := len([]rune(name)); got != 180 {
		t.Fatalf("filename rune length = %d, want 180", got)
	}
}
