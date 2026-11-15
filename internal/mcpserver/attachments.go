package mcpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/store"
)

// maxImageBytes bounds inline image attachments returned over MCP so an
// oversized upload cannot exhaust memory or exceed vision API payload limits.
const maxImageBytes = 20 << 20

const attachmentSniffBytes = 512

func (s *Server) getAttachment(ctx context.Context, st store.Store, args toolArgs) any {
	postID := strings.TrimSpace(args.PostID)
	if postID == "" {
		return toolError("post_id is required")
	}
	attachmentID := strings.TrimSpace(args.AttachmentID)
	if attachmentID == "" {
		return toolError("attachment_id is required")
	}

	thread, err := st.Threads().ByID(ctx, postID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return toolError(fmt.Sprintf("no AgentX post found with ID %q", postID))
		}
		return toolError("load post: " + err.Error())
	}

	attachment, err := st.MessageAttachments().ByID(ctx, attachmentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return toolError(fmt.Sprintf("no attachment %q found in post %q", attachmentID, postID))
		}
		return toolError("load attachment: " + err.Error())
	}
	if attachment.OrganizationID != thread.OrganizationID ||
		attachment.ConversationType != domain.ConversationThread ||
		attachment.ConversationID != thread.ID {
		return toolError(fmt.Sprintf("no attachment %q found in post %q", attachmentID, postID))
	}

	message, err := st.Messages().ByID(ctx, attachment.MessageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return toolError(fmt.Sprintf("no attachment %q found in post %q", attachmentID, postID))
		}
		return toolError("load message: " + err.Error())
	}
	if message.OrganizationID != thread.OrganizationID ||
		message.ConversationType != domain.ConversationThread ||
		message.ConversationID != thread.ID {
		return toolError(fmt.Sprintf("no attachment %q found in post %q", attachmentID, postID))
	}

	file, stat, err := s.openAttachmentFile(attachment)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return toolError(fmt.Sprintf("attachment %q (%s) content file is missing", attachment.ID, attachment.Filename))
		}
		return toolError("open attachment: " + err.Error())
	}
	defer file.Close()

	sniff := make([]byte, attachmentSniffBytes)
	n, sniffErr := io.ReadFull(file, sniff)
	if sniffErr != nil && !errors.Is(sniffErr, io.EOF) && !errors.Is(sniffErr, io.ErrUnexpectedEOF) {
		return toolError("read attachment: " + sniffErr.Error())
	}
	detectedType := normalizeMediaType(http.DetectContentType(sniff[:n]))
	storedType := normalizeMediaType(attachment.ContentType)

	if mimeType, ok := resolveImageMimeType(storedType, detectedType); ok {
		if stat.Size() > maxImageBytes {
			return toolError(fmt.Sprintf("image attachment %q (%s) is %d bytes, exceeding the %d-byte MCP limit",
				attachment.ID, attachment.Filename, stat.Size(), maxImageBytes))
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return toolError("read attachment: " + err.Error())
		}
		data, err := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
		if err != nil {
			return toolError("read attachment: " + err.Error())
		}
		if int64(len(data)) > maxImageBytes {
			return toolError(fmt.Sprintf("image attachment %q (%s) exceeds the %d-byte MCP limit",
				attachment.ID, attachment.Filename, maxImageBytes))
		}
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("Attachment `%s`: %s (%s, %d bytes)", attachment.ID, attachment.Filename, mimeType, len(data)),
				},
				{
					"type":     "image",
					"data":     base64.StdEncoding.EncodeToString(data),
					"mimeType": mimeType,
				},
			},
		}
	}

	displayType := storedType
	if displayType == "" {
		displayType = detectedType
	}
	if displayType == "" {
		displayType = "application/octet-stream"
	}

	if attachment.Kind == domain.MessageAttachmentText || storedType == "image/svg+xml" {
		return readTextAttachment(file, stat.Size(), thread.ID, attachment, displayType, args.Offset)
	}

	return toolError(fmt.Sprintf("attachment %q (%s, %s, %d bytes) is a binary file; get_attachment only supports image and text attachments",
		attachment.ID, attachment.Filename, displayType, stat.Size()))
}

func readTextAttachment(file *os.File, totalSize int64, postID string, attachment domain.MessageAttachment, displayType string, rawOffset *int) any {
	if displayType == "" || displayType == "application/octet-stream" {
		displayType = "text/plain"
	}
	if totalSize == 0 {
		return toolText(fmt.Sprintf("Attachment `%s`: %s (%s, 0 bytes)\n", attachment.ID, attachment.Filename, displayType))
	}

	offset := int64(0)
	if rawOffset != nil && *rawOffset > 0 {
		offset = int64(*rawOffset)
	}
	if offset >= totalSize {
		return toolText(fmt.Sprintf("Attachment `%s`: %s (%s, %d bytes)\n\n(offset %d is at or past the end of the %d-byte attachment.)\n",
			attachment.ID, attachment.Filename, displayType, totalSize, offset, totalSize))
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return toolError("read attachment: " + err.Error())
	}

	buf, err := io.ReadAll(io.LimitReader(file, maxPageBytes+utf8.UTFMax*2+1))
	if err != nil {
		return toolError("read attachment: " + err.Error())
	}
	if offset > 0 {
		skip := 0
		for skip < len(buf) && skip < utf8.UTFMax-1 && !utf8.RuneStart(buf[skip]) {
			skip++
		}
		offset += int64(skip)
		buf = buf[skip:]
	}
	if len(buf) == 0 {
		return toolText(fmt.Sprintf("Attachment `%s`: %s (%s, %d bytes)\n\n(offset %d is at or past the end of the %d-byte attachment.)\n",
			attachment.ID, attachment.Filename, displayType, totalSize, offset, totalSize))
	}

	chunk := cutUTF8Bytes(buf, maxPageBytes)
	if len(chunk) == 0 {
		_, size := utf8.DecodeRune(buf)
		chunk = buf[:size]
	}
	if bytes.IndexByte(chunk, 0) >= 0 || !utf8.Valid(chunk) {
		return toolError(fmt.Sprintf("attachment %q (%s, %s, %d bytes) is a binary file; get_attachment only supports image and text attachments",
			attachment.ID, attachment.Filename, displayType, totalSize))
	}

	nextOffset := offset + int64(len(chunk))
	var b strings.Builder
	if offset > 0 || nextOffset < totalSize {
		fmt.Fprintf(&b, "Attachment `%s`: %s (%s, bytes %d-%d of %d)\n\n",
			attachment.ID, attachment.Filename, displayType, offset, nextOffset, totalSize)
	} else {
		fmt.Fprintf(&b, "Attachment `%s`: %s (%s, %d bytes)\n\n",
			attachment.ID, attachment.Filename, displayType, totalSize)
	}
	b.Write(chunk)
	if nextOffset < totalSize {
		if chunk[len(chunk)-1] != '\n' {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "\n---\n\n(Attachment truncated at byte %d of %d. Call get_attachment with post_id=%q, attachment_id=%q, offset=%d to read the rest.)\n",
			nextOffset, totalSize, postID, attachment.ID, nextOffset)
	}
	return toolText(b.String())
}

func cutUTF8Bytes(buf []byte, limit int) []byte {
	if len(buf) <= limit {
		return buf
	}
	if limit <= 0 {
		return nil
	}
	for limit > 0 && !utf8.RuneStart(buf[limit]) {
		limit--
	}
	return buf[:limit]
}

func resolveImageMimeType(storedType string, detectedType string) (string, bool) {
	if isSupportedImageType(detectedType) {
		return detectedType, true
	}
	if detectedType == "application/octet-stream" && isSupportedImageType(storedType) {
		return storedType, true
	}
	return "", false
}

func isSupportedImageType(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	default:
		return false
	}
}

func normalizeMediaType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	if parsed, _, err := mime.ParseMediaType(value); err == nil {
		value = strings.ToLower(parsed)
	} else if before, _, ok := strings.Cut(value, ";"); ok {
		value = strings.TrimSpace(before)
	}
	if value == "image/jpg" {
		return "image/jpeg"
	}
	return value
}

func (s *Server) openAttachmentFile(attachment domain.MessageAttachment) (*os.File, os.FileInfo, error) {
	candidates := s.attachmentCandidatePaths(attachment)
	if len(candidates) == 0 {
		return nil, nil, os.ErrNotExist
	}
	var lastErr error
	for _, candidate := range candidates {
		file, err := os.Open(candidate)
		if err != nil {
			if lastErr == nil || !errors.Is(err, os.ErrNotExist) {
				lastErr = err
			}
			continue
		}
		stat, err := file.Stat()
		if err != nil {
			_ = file.Close()
			lastErr = err
			continue
		}
		if stat.IsDir() {
			_ = file.Close()
			lastErr = os.ErrNotExist
			continue
		}
		return file, stat, nil
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return nil, nil, lastErr
}

func (s *Server) attachmentCandidatePaths(attachment domain.MessageAttachment) []string {
	storagePath := strings.TrimSpace(attachment.StoragePath)
	if storagePath == "" {
		return nil
	}
	dataDir := strings.TrimSpace(s.dataDir)
	canonicalRel := filepath.Join(
		"attachments",
		safePathSegment(attachment.OrganizationID),
		safePathSegment(string(attachment.ConversationType)),
		safePathSegment(attachment.ConversationID),
		safePathSegment(attachment.MessageID),
		filepath.Base(storagePath),
	)

	var candidates []string
	add := func(path string) {
		if path == "" {
			return
		}
		for _, existing := range candidates {
			if existing == path {
				return
			}
		}
		candidates = append(candidates, path)
	}

	if filepath.IsAbs(storagePath) {
		add(storagePath)
		if dataDir != "" {
			add(filepath.Join(dataDir, canonicalRel))
		}
		return candidates
	}

	if dataDir != "" {
		add(filepath.Join(dataDir, canonicalRel))
		cleanData := filepath.Clean(dataDir)
		if cleanData != "." && cleanData != string(filepath.Separator) {
			prefix := filepath.Base(cleanData) + string(filepath.Separator)
			if trimmed, ok := strings.CutPrefix(filepath.Clean(storagePath), prefix); ok {
				if joined := filepath.Join(cleanData, trimmed); isWithinDir(cleanData, joined) {
					add(joined)
				}
			}
		}
		if joined := filepath.Join(dataDir, storagePath); isWithinDir(dataDir, joined) {
			add(joined)
		}
		return candidates
	}

	add(storagePath)
	return candidates
}

func isWithinDir(dir string, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(target))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
