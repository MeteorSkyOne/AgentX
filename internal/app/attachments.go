package app

import (
	"bytes"
	"context"
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

const (
	MaxMessageAttachments          = 5
	MaxAttachmentBytes             = 10 * 1024 * 1024
	MaxMessageAttachmentTotalBytes = 25 * 1024 * 1024
)

type AttachmentUpload struct {
	Filename    string
	ContentType string
	Data        []byte
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

	var total int64
	classified := make([]attachmentClassification, 0, len(uploads))
	for _, upload := range uploads {
		size := int64(len(upload.Data))
		if size == 0 {
			return nil, invalidInput("empty attachment")
		}
		if size > MaxAttachmentBytes {
			return nil, invalidInput("attachment exceeds 10 MiB")
		}
		total += size
		if total > MaxMessageAttachmentTotalBytes {
			return nil, invalidInput("attachments exceed 25 MiB total")
		}
		info, err := classifyAttachment(upload)
		if err != nil {
			return nil, err
		}
		classified = append(classified, info)
	}

	dir := a.messageAttachmentDir(message)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	attachments := make([]domain.MessageAttachment, 0, len(uploads))
	for index, upload := range uploads {
		info := classified[index]
		attachmentID := id.New("att")
		storagePath := filepath.Join(dir, attachmentID+safeAttachmentExt(info.Filename))
		if err := os.WriteFile(storagePath, upload.Data, 0o600); err != nil {
			_ = removeAttachmentFiles(attachments)
			return nil, err
		}
		attachments = append(attachments, domain.MessageAttachment{
			ID:               attachmentID,
			MessageID:        message.ID,
			OrganizationID:   message.OrganizationID,
			ConversationType: message.ConversationType,
			ConversationID:   message.ConversationID,
			Filename:         info.Filename,
			ContentType:      info.ContentType,
			Kind:             info.Kind,
			SizeBytes:        int64(len(upload.Data)),
			StoragePath:      storagePath,
			CreatedAt:        message.CreatedAt.Add(time.Duration(index)),
		})
	}
	return attachments, nil
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

func classifyAttachment(upload AttachmentUpload) (attachmentClassification, error) {
	filename := sanitizeAttachmentFilename(upload.Filename)
	headerType := mediaType(upload.ContentType)
	detectedType := mediaType(http.DetectContentType(sniffBytes(upload.Data)))
	ext := strings.ToLower(filepath.Ext(filename))

	if ext == ".svg" || headerType == "image/svg+xml" || detectedType == "image/svg+xml" {
		return attachmentClassification{}, invalidInput("SVG attachments are not supported")
	}

	if isAllowedImageType(headerType) || isAllowedImageType(detectedType) {
		contentType := detectedType
		if contentType == "" {
			contentType = headerType
		}
		if !isAllowedImageType(contentType) {
			return attachmentClassification{}, invalidInput("unsupported image attachment type")
		}
		return attachmentClassification{
			Filename:    filename,
			ContentType: contentType,
			Kind:        domain.MessageAttachmentImage,
		}, nil
	}

	if isUTF8Text(upload.Data) && isTextAttachmentType(headerType, detectedType, ext) {
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
		}, nil
	}

	return attachmentClassification{}, invalidInput("unsupported attachment type")
}

func sniffBytes(data []byte) []byte {
	if len(data) > 512 {
		return data[:512]
	}
	return data
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

func isUTF8Text(data []byte) bool {
	return utf8.Valid(data) && !bytes.Contains(data, []byte{0})
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
