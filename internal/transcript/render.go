package transcript

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/meteorsky/agentx/internal/domain"
)

const (
	processTextLimit = 600
	snippetRadius    = 160
)

type RenderOptions struct {
	// IncludeProcess adds agent thinking and tool calls under each reply.
	IncludeProcess bool
	// Offset skips that many messages; Limit caps how many are rendered (0 = all).
	Offset int
	Limit  int
	// MaxBytes stops adding messages once the page body would exceed this
	// many bytes (0 = unbounded). The first message is always included and is
	// cut to fit when it alone is larger.
	MaxBytes int
	// Resume continues the first selected message from this byte offset of its
	// rendered text, as reported by Page.Resume for a truncated message.
	Resume int
}

// Page is one rendered slice of a post.
type Page struct {
	Text string
	// First and Last are the 1-based indexes of the rendered messages; both
	// are zero when no message falls inside the window.
	First int
	Last  int
	// Truncated reports that the last rendered message was cut to MaxBytes;
	// Resume is then the offset to pass back to read the rest of it.
	Truncated bool
	Resume    int
}

// Markdown renders the post header followed by the selected messages.
func Markdown(post Post, opts RenderOptions) string {
	return Render(post, opts).Text
}

// Render renders the post header and as many selected messages as fit.
func Render(post Post, opts RenderOptions) Page {
	offset := max(opts.Offset, 0)
	end := len(post.Messages)
	if opts.Limit > 0 && offset+opts.Limit < end {
		end = offset + opts.Limit
	}
	indexByID := make(map[string]int, len(post.Messages))
	for _, message := range post.Messages {
		indexByID[message.ID] = message.Index
	}

	var page Page
	var body strings.Builder
	for i := offset; i < end; i++ {
		message := post.Messages[i]
		var entry strings.Builder
		entry.WriteString("\n---\n\n")
		writeMessage(&entry, message, indexByID, opts.IncludeProcess)
		content := entry.String()
		start := 0
		prefix := ""
		if i == offset && opts.Resume > 0 && opts.Resume < len(content) {
			start = opts.Resume
			for start < len(content) && !utf8.RuneStart(content[start]) {
				start++
			}
			prefix = fmt.Sprintf("\n---\n\n## [%d] %s (%s), continued\n\n", message.Index, message.SenderName, message.SenderType)
		}
		text := prefix + content[start:]
		if opts.MaxBytes > 0 && body.Len()+len(text) > opts.MaxBytes {
			if page.First != 0 {
				break
			}
			kept := cutUTF8(content[start:], opts.MaxBytes-len(prefix))
			if kept == "" {
				_, size := utf8.DecodeRuneInString(content[start:])
				kept = content[start : start+size]
			}
			text = prefix + kept + "\n\n[message truncated]\n"
			page.Truncated = true
			page.Resume = start + len(kept)
		}
		body.WriteString(text)
		if page.First == 0 {
			page.First = message.Index
		}
		page.Last = message.Index
		if page.Truncated {
			break
		}
	}

	var b strings.Builder
	title := strings.TrimSpace(post.Title)
	if title == "" {
		title = "Untitled post"
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "- Post ID: `%s`\n", post.ID)
	if post.ProjectName != "" {
		fmt.Fprintf(&b, "- Project: %s\n", post.ProjectName)
	}
	if post.ChannelName != "" {
		fmt.Fprintf(&b, "- Channel: #%s\n", post.ChannelName)
	}
	fmt.Fprintf(&b, "- Created: %s\n", formatTime(post.CreatedAt))
	fmt.Fprintf(&b, "- Updated: %s\n", formatTime(post.UpdatedAt))
	if post.ArchivedAt != nil {
		fmt.Fprintf(&b, "- Deleted: %s\n", formatTime(*post.ArchivedAt))
	}
	fmt.Fprintf(&b, "- Messages: %d\n", len(post.Messages))
	if page.First != 0 && (page.First > 1 || page.Last < len(post.Messages)) {
		fmt.Fprintf(&b, "- Showing: messages %d-%d\n", page.First, page.Last)
	}
	b.WriteString(body.String())
	page.Text = b.String()
	return page
}

// cutUTF8 shortens s to at most limit bytes without splitting a rune.
func cutUTF8(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	if limit <= 0 {
		return ""
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit]
}

func writeMessage(b *strings.Builder, message Message, indexByID map[string]int, includeProcess bool) {
	fmt.Fprintf(b, "## [%d] %s (%s) - %s\n\n", message.Index, message.SenderName, message.SenderType, formatTime(message.CreatedAt))
	if message.ReplyToID != "" {
		if index, ok := indexByID[message.ReplyToID]; ok {
			fmt.Fprintf(b, "_Reply to [%d]_\n\n", index)
		} else {
			b.WriteString("_Reply to a deleted message_\n\n")
		}
	}
	if body := strings.TrimSpace(message.Body); body != "" {
		b.WriteString(body)
		b.WriteString("\n")
	}
	if len(message.Attachments) > 0 {
		b.WriteString("\nAttachments:\n")
		for _, attachment := range message.Attachments {
			fmt.Fprintf(b, "- %s (%s, %d bytes)\n", attachment.Filename, attachment.ContentType, attachment.SizeBytes)
		}
	}
	if !includeProcess {
		return
	}
	items := message.ProcessItems()
	if len(items) == 0 {
		return
	}
	b.WriteString("\n<details><summary>Process</summary>\n\n")
	for _, item := range items {
		if item.Type == "thinking" {
			// Reasoning is kept whole; it is what a reader usually wants from the process.
			if text := strings.TrimSpace(item.Text); text != "" {
				fmt.Fprintf(b, "\n_Thinking:_\n\n%s\n\n", text)
			}
			continue
		}
		if line := processLine(item); line != "" {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n</details>\n")
}

// ProcessItems mirrors the web UI: the recorded process list already carries
// the reply's thinking, so the standalone thinking field is only a fallback
// for replies stored without a process list.
func (m Message) ProcessItems() []domain.ProcessItem {
	if len(m.Process) > 0 {
		return m.Process
	}
	if m.Thinking != "" {
		return []domain.ProcessItem{{Type: "thinking", Text: m.Thinking}}
	}
	return nil
}

// processSearchText flattens process items, untruncated, for keyword search.
func processSearchText(items []domain.ProcessItem) string {
	var b strings.Builder
	for _, item := range items {
		for _, part := range []string{item.ToolName, item.Text, fullValue(item.Input), fullValue(item.Output)} {
			if part = strings.TrimSpace(part); part != "" {
				b.WriteString("\n")
				b.WriteString(part)
			}
		}
	}
	return b.String()
}

// fullValue joins the string leaves of a tool input or output so searches and
// snippets see the text as the agent wrote it, not as escaped JSON.
func fullValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text := fullValue(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(v))
		for _, key := range keys {
			if text := fullValue(v[key]); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprint(v)
	}
}

func processLine(item domain.ProcessItem) string {
	switch item.Type {
	case "tool_call":
		line := fmt.Sprintf("- tool `%s`", item.ToolName)
		if item.Status != "" {
			line += " (" + item.Status + ")"
		}
		if summary := compactValue(item.Input); summary != "" {
			line += ": " + summary
		}
		return line
	case "tool_result":
		summary := compactValue(item.Output)
		if summary == "" {
			summary = truncate(oneLine(item.Text), processTextLimit)
		}
		if summary == "" {
			return ""
		}
		return "  - result: " + summary
	default:
		summary := truncate(oneLine(item.Text), processTextLimit)
		if summary == "" {
			return ""
		}
		return fmt.Sprintf("- %s: %s", item.Type, summary)
	}
}

func compactValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return truncate(oneLine(v), processTextLimit)
	default:
		var encoded strings.Builder
		encoder := json.NewEncoder(&encoded)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(v); err != nil {
			return ""
		}
		return truncate(strings.TrimSpace(encoded.String()), processTextLimit)
	}
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

type Match struct {
	Message Message `json:"-"`
	Snippet string  `json:"snippet"`
	// InProcess reports that the snippet comes from thinking or tool calls
	// rather than the message body.
	InProcess bool `json:"in_process,omitempty"`
}

type SearchOptions struct {
	// IncludeProcess also searches agent thinking and tool calls.
	IncludeProcess bool
	Limit          int
}

// Search returns messages whose body or attachment names (and, optionally,
// process) contain every whitespace-separated term of query, compared
// case-insensitively.
func Search(post Post, query string, opts SearchOptions) []Match {
	var terms [][]rune
	for _, term := range strings.Fields(query) {
		terms = append(terms, lowerRunes([]rune(term)))
	}
	if len(terms) == 0 {
		return nil
	}
	var matches []Match
	for _, message := range post.Messages {
		text := message.Body
		for _, attachment := range message.Attachments {
			text += "\n" + attachment.Filename
		}
		visibleLen := utf8.RuneCountInString(text)
		if opts.IncludeProcess {
			text += processSearchText(message.ProcessItems())
		}
		runes := []rune(text)
		lowered := lowerRunes(runes)
		first := -1
		for _, term := range terms {
			at := runeIndex(lowered, term)
			if at < 0 {
				first = -1
				break
			}
			if first < 0 || at < first {
				first = at
			}
		}
		if first < 0 {
			continue
		}
		matches = append(matches, Match{Message: message, Snippet: snippet(runes, first), InProcess: first >= visibleLen})
		if opts.Limit > 0 && len(matches) >= opts.Limit {
			break
		}
	}
	return matches
}

func snippet(runes []rune, at int) string {
	start := max(at-snippetRadius, 0)
	end := min(at+snippetRadius, len(runes))
	text := oneLine(string(runes[start:end]))
	if start > 0 {
		text = "..." + text
	}
	if end < len(runes) {
		text += "..."
	}
	return text
}

func lowerRunes(runes []rune) []rune {
	lowered := make([]rune, len(runes))
	for i, r := range runes {
		lowered[i] = unicode.ToLower(r)
	}
	return lowered
}

func runeIndex(haystack []rune, needle []rune) int {
	if len(needle) == 0 {
		return 0
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		found := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				found = false
				break
			}
		}
		if found {
			return i
		}
	}
	return -1
}
