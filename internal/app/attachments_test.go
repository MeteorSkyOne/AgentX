package app

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"
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

func TestPrepareMessageAttachmentsStreamsUploads(t *testing.T) {
	dir := t.TempDir()
	application := &App{opts: Options{DataDir: dir}}
	body := strings.Repeat("héllo wörld ", 20000) // spans several io.Copy chunks
	message := domain.Message{
		ID:               "msg_stream",
		OrganizationID:   "org_stream",
		ConversationType: domain.ConversationChannel,
		ConversationID:   "chn_stream",
		CreatedAt:        time.Now().UTC(),
	}

	attachments, err := application.prepareMessageAttachments(message, []AttachmentUpload{{
		Filename:    "notes.txt",
		ContentType: "text/plain",
		// One byte per Read, so multi-byte runes straddle scanner writes.
		Open: func() (io.ReadCloser, error) {
			return io.NopCloser(iotest.OneByteReader(strings.NewReader(body))), nil
		},
	}})
	if err != nil {
		t.Fatalf("prepareMessageAttachments error = %v", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(attachments))
	}
	stored := attachments[0]
	if stored.Kind != domain.MessageAttachmentText {
		t.Fatalf("kind = %q, want %q", stored.Kind, domain.MessageAttachmentText)
	}
	if stored.SizeBytes != int64(len(body)) {
		t.Fatalf("size = %d, want %d", stored.SizeBytes, len(body))
	}
	contents, err := os.ReadFile(stored.StoragePath)
	if err != nil {
		t.Fatalf("read stored attachment: %v", err)
	}
	if string(contents) != body {
		t.Fatalf("stored contents differ from upload")
	}
}

func TestPrepareMessageAttachmentsClassifiesBinaryUploadAsFile(t *testing.T) {
	application := &App{opts: Options{DataDir: t.TempDir()}}
	attachments, err := application.prepareMessageAttachments(domain.Message{
		ID:               "msg_binary",
		OrganizationID:   "org_binary",
		ConversationType: domain.ConversationChannel,
		ConversationID:   "chn_binary",
		CreatedAt:        time.Now().UTC(),
	}, []AttachmentUpload{{
		Filename:    "blob.txt",
		ContentType: "text/plain",
		Data:        append([]byte("prefix"), 0x00, 0x01, 0x02),
	}})
	if err != nil {
		t.Fatalf("prepareMessageAttachments error = %v", err)
	}
	if attachments[0].Kind != domain.MessageAttachmentFile {
		t.Fatalf("kind = %q, want %q", attachments[0].Kind, domain.MessageAttachmentFile)
	}
}

func TestPrepareMessageAttachmentsCleansUpAfterEmptyStream(t *testing.T) {
	dir := t.TempDir()
	application := &App{opts: Options{DataDir: dir}}
	message := domain.Message{
		ID:               "msg_partial",
		OrganizationID:   "org_partial",
		ConversationType: domain.ConversationChannel,
		ConversationID:   "chn_partial",
		CreatedAt:        time.Now().UTC(),
	}

	_, err := application.prepareMessageAttachments(message, []AttachmentUpload{
		{Filename: "first.txt", ContentType: "text/plain", Data: []byte("kept?")},
		{Filename: "second.txt", ContentType: "text/plain", Data: nil},
	})
	if !errors.Is(err, ErrInvalidInput) || InvalidInputMessage(err) != "empty attachment" {
		t.Fatalf("error = %v, want empty attachment invalid input", err)
	}
	if _, statErr := os.Stat(application.messageAttachmentDir(message)); !os.IsNotExist(statErr) {
		t.Fatalf("attachment dir stat = %v, want not exist", statErr)
	}
}

func TestAttachmentScannerDetectsSplitRunes(t *testing.T) {
	scanner := &attachmentScanner{}
	text := []byte("日本語テキスト")
	for _, b := range text {
		if _, err := scanner.Write([]byte{b}); err != nil {
			t.Fatalf("scanner write error = %v", err)
		}
	}
	if !scanner.isText() {
		t.Fatal("isText = false, want true for UTF-8 split across writes")
	}

	truncated := &attachmentScanner{}
	if _, err := truncated.Write(text[:len(text)-1]); err != nil {
		t.Fatalf("scanner write error = %v", err)
	}
	if truncated.isText() {
		t.Fatal("isText = true, want false for a trailing incomplete rune")
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
