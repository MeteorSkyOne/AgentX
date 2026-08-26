package app

import (
	"bytes"
	"context"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
)

// MaxMessageAttachments caps how many files a single message may carry.
// Attachment size is intentionally unlimited.
const MaxMessageAttachments = 5

type AttachmentUpload struct {
	Filename    string
	ContentType string
	// Data holds the whole attachment for in-process callers. It is ignored
	// when Open is set.
	Data []byte
	// Open, when set, yields the attachment contents as a stream so uploads are
	// copied straight to disk instead of being buffered in memory.
	Open func() (io.ReadCloser, error)
}

func (u AttachmentUpload) open() (io.ReadCloser, error) {
	if u.Open != nil {
		return u.Open()
	}
	return io.NopCloser(bytes.NewReader(u.Data)), nil
}

func (a *App) Attachment(ctx context.Context, attachmentID string) (domain.MessageAttachment, error) {
	return a.store.MessageAttachments().ByID(ctx, attachmentID)
}

func (a *App) prepareMessageAttachments(message domain.Message, uploads []AttachmentUpload) ([]domain.MessageAttachment, error) {
	if len(uploads) == 0 {
		return nil, nil
	}
	if len(uploads) > MaxMessageAttachments {
		return nil, invalidInput("too many attachments")
	}

	dir := a.messageAttachmentDir(message)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	attachments := make([]domain.MessageAttachment, 0, len(uploads))
	for index, upload := range uploads {
		attachmentID := id.New("att")
		filename := sanitizeAttachmentFilename(upload.Filename)
		storagePath := filepath.Join(dir, attachmentID+safeAttachmentExt(filename))

		stored, err := storeAttachmentFile(storagePath, upload)
		if err == nil && stored.size == 0 {
			err = invalidInput("empty attachment")
		}
		if err != nil {
			_ = os.Remove(storagePath)
			_ = removeAttachmentFiles(attachments)
			_ = os.Remove(dir)
			return nil, err
		}

		info := classifyAttachment(filename, upload.ContentType, stored)
		attachments = append(attachments, domain.MessageAttachment{
			ID:               attachmentID,
			MessageID:        message.ID,
			OrganizationID:   message.OrganizationID,
			ConversationType: message.ConversationType,
			ConversationID:   message.ConversationID,
			Filename:         info.Filename,
			ContentType:      info.ContentType,
			Kind:             info.Kind,
			SizeBytes:        stored.size,
			StoragePath:      storagePath,
			CreatedAt:        message.CreatedAt.Add(time.Duration(index)),
		})
	}
	return attachments, nil
}

// storedAttachment describes an attachment that has already been streamed to
// disk: everything classification needs, without holding the payload in memory.
type storedAttachment struct {
	size  int64
	sniff []byte
	text  bool
}

// storeAttachmentFile copies an upload to path, inspecting the bytes as they go
// past so the file never has to be read back or held in memory.
func storeAttachmentFile(path string, upload AttachmentUpload) (storedAttachment, error) {
	src, err := upload.open()
	if err != nil {
		return storedAttachment{}, err
	}
	defer src.Close()

	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return storedAttachment{}, err
	}
	scanner := &attachmentScanner{}
	size, copyErr := io.Copy(dst, io.TeeReader(src, scanner))
	closeErr := dst.Close()
	if copyErr != nil {
		return storedAttachment{}, copyErr
	}
	if closeErr != nil {
		return storedAttachment{}, closeErr
	}
	return storedAttachment{size: size, sniff: scanner.sniff, text: scanner.isText()}, nil
}

func (a *App) messageAttachmentDir(message domain.Message) string {
	dataDir := strings.TrimSpace(a.opts.DataDir)
	if dataDir == "" {
		dataDir = "."
	}
	return filepath.Join(
		dataDir,
		"attachments",
		safePathSegment(message.OrganizationID),
		safePathSegment(string(message.ConversationType)),
		safePathSegment(message.ConversationID),
		safePathSegment(message.ID),
	)
}

type attachmentClassification struct {
	Filename    string
	ContentType string
	Kind        domain.MessageAttachmentKind
}

func classifyAttachment(filename string, uploadContentType string, stored storedAttachment) attachmentClassification {
	headerType := mediaType(uploadContentType)
	detectedType := mediaType(http.DetectContentType(stored.sniff))
	ext := strings.ToLower(filepath.Ext(filename))

	if isAllowedImageType(detectedType) || (isAllowedImageType(headerType) && detectedType == "application/octet-stream") {
		contentType := detectedType
		if !isAllowedImageType(contentType) {
			contentType = headerType
		}
		return attachmentClassification{
			Filename:    filename,
			ContentType: contentType,
			Kind:        domain.MessageAttachmentImage,
		}
	}

	if stored.text && isTextAttachmentType(headerType, detectedType, ext) {
		contentType := headerType
		if contentType == "" || contentType == "application/octet-stream" {
			contentType = detectedType
		}
		if contentType == "" {
			contentType = "text/plain"
		}
		return attachmentClassification{
			Filename:    filename,
			ContentType: contentType,
			Kind:        domain.MessageAttachmentText,
		}
	}

	contentType := headerType
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = detectedType
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return attachmentClassification{
		Filename:    filename,
		ContentType: contentType,
		Kind:        domain.MessageAttachmentFile,
	}
}

// attachmentSniffBytes matches what http.DetectContentType inspects.
const attachmentSniffBytes = 512

// attachmentScanner inspects attachment bytes as they stream to disk: it keeps
// the leading bytes for content sniffing and tracks whether the payload so far
// is NUL-free valid UTF-8.
type attachmentScanner struct {
	sniff   []byte
	binary  bool
	partial []byte
}

func (s *attachmentScanner) Write(p []byte) (int, error) {
	if len(s.sniff) < attachmentSniffBytes {
		s.sniff = append(s.sniff, p[:min(len(p), attachmentSniffBytes-len(s.sniff))]...)
	}
	if s.binary {
		return len(p), nil
	}
	if bytes.IndexByte(p, 0) >= 0 {
		s.binary = true
		s.partial = nil
		return len(p), nil
	}

	buf := p
	if len(s.partial) > 0 {
		buf = append(s.partial, p...)
		s.partial = nil
	}
	complete, partial := splitTrailingRune(buf)
	if !utf8.Valid(complete) {
		s.binary = true
		return len(p), nil
	}
	if len(partial) > 0 {
		s.partial = append([]byte(nil), partial...)
	}
	return len(p), nil
}

// isText reports whether every byte written formed NUL-free valid UTF-8. A
// leftover partial rune means the payload ended mid-sequence, which is not.
func (s *attachmentScanner) isText() bool {
	return !s.binary && len(s.partial) == 0
}

// splitTrailingRune splits buf just before a trailing incomplete UTF-8 rune so
// that a multi-byte rune straddling two writes is validated as a whole.
func splitTrailingRune(buf []byte) (complete []byte, partial []byte) {
	for i := 1; i <= utf8.UTFMax && i <= len(buf); i++ {
		b := buf[len(buf)-i]
		if !utf8.RuneStart(b) {
			continue
		}
		if runeByteLen(b) > i {
			return buf[:len(buf)-i], buf[len(buf)-i:]
		}
		return buf, nil
	}
	return buf, nil
}

// runeByteLen returns the encoded length of the rune starting with b, or 1 for
// bytes that cannot start one so utf8.Valid gets to reject them.
func runeByteLen(b byte) int {
	switch {
	case b < 0x80:
		return 1
	case b&0xE0 == 0xC0:
		return 2
	case b&0xF0 == 0xE0:
		return 3
	case b&0xF8 == 0xF0:
		return 4
	default:
		return 1
	}
}

func isAllowedImageType(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	default:
		return false
	}
}

func isTextAttachmentType(headerType string, detectedType string, ext string) bool {
	return strings.HasPrefix(headerType, "text/") ||
		strings.HasPrefix(detectedType, "text/") ||
		isKnownTextContentType(headerType) ||
		isKnownTextExtension(ext)
}

func isKnownTextContentType(contentType string) bool {
	switch contentType {
	case "application/json", "application/x-ndjson", "application/xml", "application/yaml", "application/x-yaml", "application/toml", "application/javascript":
		return true
	default:
		return false
	}
}

func isKnownTextExtension(ext string) bool {
	switch ext {
	case ".txt", ".md", ".markdown", ".json", ".jsonl", ".csv", ".log", ".yaml", ".yml", ".toml", ".xml", ".html", ".css", ".js", ".jsx", ".ts", ".tsx", ".go", ".py", ".rs", ".java", ".c", ".cc", ".cpp", ".h", ".hpp", ".sh", ".sql", ".env", ".ini", ".conf", ".gitignore":
		return true
	default:
		return false
	}
}

func mediaType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	parsed, _, err := mime.ParseMediaType(value)
	if err == nil {
		return strings.ToLower(parsed)
	}
	if before, _, ok := strings.Cut(value, ";"); ok {
		return strings.TrimSpace(before)
	}
	return value
}

func sanitizeAttachmentFilename(filename string) string {
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "." || filename == string(filepath.Separator) || filename == "" {
		filename = "attachment"
	}
	filename = strings.Map(func(r rune) rune {
		switch {
		case r < 32 || r == 127:
			return -1
		case r == '/' || r == '\\':
			return '_'
		default:
			return r
		}
	}, filename)
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return "attachment"
	}
	runes := []rune(filename)
	if len(runes) > 180 {
		ext := filepath.Ext(filename)
		extRunes := []rune(ext)
		maxBase := 180 - len(extRunes)
		if maxBase < 1 {
			maxBase = 1
		}
		filename = string(runes[:maxBase]) + ext
	}
	return filename
}

func safeAttachmentExt(filename string) string {
	ext := filepath.Ext(filename)
	if ext == "" || len(ext) > 20 || strings.ContainsAny(ext, `/\`) {
		return ""
	}
	return ext
}

func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "_"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	if b.Len() == 0 {
		return "_"
	}
	return b.String()
}

func removeAttachmentFiles(attachments []domain.MessageAttachment) error {
	var firstErr error
	seenDirs := make(map[string]bool)
	for _, attachment := range attachments {
		if attachment.StoragePath == "" {
			continue
		}
		if err := os.Remove(attachment.StoragePath); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = err
		}
		seenDirs[filepath.Dir(attachment.StoragePath)] = true
	}
	for dir := range seenDirs {
		_ = os.Remove(dir)
	}
	return firstErr
}
