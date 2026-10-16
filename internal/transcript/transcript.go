// Package transcript turns a forum post (thread) and its messages into a
// self-contained, read-only record that can be exported or handed to another
// agent without resuming the original agent session.
package transcript

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/store"
)

// ErrNotFound is returned when no post exists for the requested ID.
var ErrNotFound = errors.New("post not found")

// maxMessages bounds how many messages a single transcript loads.
const maxMessages = 100000

type Post struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	ProjectID   string     `json:"project_id"`
	ProjectName string     `json:"project_name,omitempty"`
	ChannelID   string     `json:"channel_id"`
	ChannelName string     `json:"channel_name,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	Messages    []Message  `json:"messages"`
}

type Message struct {
	// Index is the 1-based position of the message within the post.
	Index       int                  `json:"index"`
	ID          string               `json:"id"`
	SenderType  domain.SenderType    `json:"sender_type"`
	SenderName  string               `json:"sender_name"`
	Kind        domain.MessageKind   `json:"kind"`
	Body        string               `json:"body"`
	ReplyToID   string               `json:"reply_to_message_id,omitempty"`
	Thinking    string               `json:"thinking,omitempty"`
	Process     []domain.ProcessItem `json:"process,omitempty"`
	Attachments []Attachment         `json:"attachments,omitempty"`
	CreatedAt   time.Time            `json:"created_at"`
}

type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes"`
}

// Load reads a post and all of its messages, resolving sender display names.
func Load(ctx context.Context, st store.Store, postID string) (Post, error) {
	postID = strings.TrimSpace(postID)
	if postID == "" {
		return Post{}, ErrNotFound
	}
	thread, err := st.Threads().ByID(ctx, postID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Post{}, ErrNotFound
		}
		return Post{}, err
	}
	post := Post{
		ID:         thread.ID,
		Title:      thread.Title,
		ProjectID:  thread.ProjectID,
		ChannelID:  thread.ChannelID,
		CreatedAt:  thread.CreatedAt,
		UpdatedAt:  thread.UpdatedAt,
		ArchivedAt: thread.ArchivedAt,
	}
	if project, err := st.Projects().ByID(ctx, thread.ProjectID); err == nil {
		post.ProjectName = project.Name
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Post{}, err
	}
	if channel, err := st.Channels().ByID(ctx, thread.ChannelID); err == nil {
		post.ChannelName = channel.Name
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Post{}, err
	}

	names, err := newSenderNames(ctx, st, thread.OrganizationID)
	if err != nil {
		return Post{}, err
	}
	messages, err := st.Messages().List(ctx, domain.ConversationThread, thread.ID, maxMessages)
	if err != nil {
		return Post{}, err
	}
	post.Messages = make([]Message, 0, len(messages))
	for i, message := range messages {
		attachments, err := st.MessageAttachments().ListByMessage(ctx, message.ID)
		if err != nil {
			return Post{}, err
		}
		name, err := names.name(ctx, message.SenderType, message.SenderID)
		if err != nil {
			return Post{}, err
		}
		entry := Message{
			Index:      i + 1,
			ID:         message.ID,
			SenderType: message.SenderType,
			SenderName: name,
			Kind:       message.Kind,
			Body:       cleanBody(message.Body),
			ReplyToID:  strings.TrimSpace(message.ReplyToMessageID),
			Thinking:   metadataString(message.Metadata, "thinking"),
			Process:    metadataProcess(message.Metadata),
			CreatedAt:  message.CreatedAt,
		}
		for _, attachment := range attachments {
			entry.Attachments = append(entry.Attachments, Attachment{
				Filename:    attachment.Filename,
				ContentType: attachment.ContentType,
				SizeBytes:   attachment.SizeBytes,
			})
		}
		post.Messages = append(post.Messages, entry)
	}
	return post, nil
}

type senderNames struct {
	st     store.Store
	agents map[string]string
	users  map[string]string
}

func newSenderNames(ctx context.Context, st store.Store, orgID string) (*senderNames, error) {
	agents, err := st.Agents().ListByOrganization(ctx, orgID)
	if err != nil {
		return nil, err
	}
	names := &senderNames{st: st, agents: map[string]string{}, users: map[string]string{}}
	for _, agent := range agents {
		label := agent.Name
		if agent.Handle != "" {
			label = fmt.Sprintf("%s (@%s)", agent.Name, agent.Handle)
		}
		names.agents[agent.BotUserID] = label
	}
	return names, nil
}

func (n *senderNames) name(ctx context.Context, senderType domain.SenderType, senderID string) (string, error) {
	switch senderType {
	case domain.SenderBot:
		if label, ok := n.agents[senderID]; ok {
			return label, nil
		}
		return "Agent", nil
	case domain.SenderUser:
		if label, ok := n.users[senderID]; ok {
			return label, nil
		}
		label := "User"
		user, err := n.st.Users().ByID(ctx, senderID)
		if err == nil && strings.TrimSpace(user.DisplayName) != "" {
			label = user.DisplayName
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		n.users[senderID] = label
		return label, nil
	default:
		return "System", nil
	}
}

// processBreakPattern matches the markers runtimes insert into reply bodies so
// the UI can interleave process items; they carry no meaning in a transcript.
var processBreakPattern = regexp.MustCompile(`(\s*<!-- process-break:\d+ -->\s*)+`)

func cleanBody(body string) string {
	return strings.TrimSpace(processBreakPattern.ReplaceAllString(body, "\n\n"))
}

func metadataString(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func metadataProcess(metadata map[string]any) []domain.ProcessItem {
	raw, ok := metadata["process"]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var items []domain.ProcessItem
	if err := json.Unmarshal(encoded, &items); err != nil {
		return nil
	}
	return items
}
