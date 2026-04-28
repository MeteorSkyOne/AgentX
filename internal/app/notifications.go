package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
)

const AgentMessageCreatedWebhookEvent = "agent.message.created"

const defaultWebhookTimeout = 5 * time.Second
const webhookURLBodyLimit = 180

var ErrWebhookDeliveryFailed = errors.New("webhook delivery failed")

type NotificationSettingsUpdateRequest struct {
	WebhookEnabled bool
	WebhookURL     string
	WebhookSecret  *string
}

type AgentMessageWebhookPayload struct {
	Event            string                  `json:"event"`
	Delivery         string                  `json:"delivery"`
	Title            string                  `json:"title,omitempty"`
	OrganizationID   string                  `json:"organization_id"`
	ConversationType domain.ConversationType `json:"conversation_type,omitempty"`
	ConversationID   string                  `json:"conversation_id,omitempty"`
	Message          domain.Message          `json:"message"`
	Test             bool                    `json:"test,omitempty"`
	CreatedAt        time.Time               `json:"created_at"`
}

func (a *App) NotificationSettings(ctx context.Context, orgID string) (domain.NotificationSettings, error) {
	settings, err := a.store.NotificationSettings().ByOrganization(ctx, orgID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return defaultNotificationSettings(orgID), nil
		}
		return domain.NotificationSettings{}, err
	}
	return redactNotificationSettings(settings), nil
}

func (a *App) UpdateNotificationSettings(ctx context.Context, orgID string, req NotificationSettingsUpdateRequest) (domain.NotificationSettings, error) {
	current, err := a.store.NotificationSettings().ByOrganization(ctx, orgID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			current = defaultNotificationSettings(orgID)
		} else {
			return domain.NotificationSettings{}, err
		}
	}

	webhookURL, err := validateWebhookURL(req.WebhookURL, req.WebhookEnabled)
	if err != nil {
		return domain.NotificationSettings{}, err
	}
	current.WebhookEnabled = req.WebhookEnabled
	current.WebhookURL = webhookURL
	if req.WebhookSecret != nil {
		current.WebhookSecret = strings.TrimSpace(*req.WebhookSecret)
	}
	now := time.Now().UTC()
	if current.CreatedAt.IsZero() {
		current.CreatedAt = now
	}
	current.UpdatedAt = now
	current.WebhookSecretConfigured = current.WebhookSecret != ""

	if err := a.store.NotificationSettings().Upsert(ctx, current); err != nil {
		return domain.NotificationSettings{}, err
	}
	return redactNotificationSettings(current), nil
}

func (a *App) TestNotificationSettings(ctx context.Context, orgID string) error {
	settings, err := a.store.NotificationSettings().ByOrganization(ctx, orgID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidInput
		}
		return err
	}
	if _, err := validateWebhookURL(settings.WebhookURL, true); err != nil {
		return err
	}
	now := time.Now().UTC()
	message := domain.Message{
		ID:               id.New("msg"),
		OrganizationID:   orgID,
		ConversationType: domain.ConversationChannel,
		ConversationID:   "notification-test",
		SenderType:       domain.SenderBot,
		SenderID:         "agentx",
		Kind:             domain.MessageText,
		Body:             "AgentX notification test",
		CreatedAt:        now,
	}
	payload := AgentMessageWebhookPayload{
		Event:            AgentMessageCreatedWebhookEvent,
		Title:            "AgentX",
		OrganizationID:   orgID,
		ConversationType: message.ConversationType,
		ConversationID:   message.ConversationID,
		Message:          message,
		Test:             true,
		CreatedAt:        now,
	}
	return a.postWebhook(ctx, settings, payload)
}

func (a *App) notifyAgentMessageCreated(ctx context.Context, title string, message domain.Message) {
	if isTeamDiscussionMessage(message) {
		return
	}
	go func() {
		if err := a.deliverAgentMessageWebhook(ctx, title, message); err != nil {
			log.Printf("agentx webhook delivery failed org=%s message=%s: %v", message.OrganizationID, message.ID, err)
		}
	}()
}

func isTeamDiscussionMessage(message domain.Message) bool {
	team, ok := message.Metadata["team"]
	if !ok {
		return false
	}
	switch value := team.(type) {
	case domain.TeamMetadata:
		return value.SessionID != "" && value.Phase != "summary"
	case map[string]any:
		sessionID, _ := value["session_id"].(string)
		phase, _ := value["phase"].(string)
		return sessionID != "" && phase != "summary"
	default:
		return false
	}
}

func (a *App) deliverAgentMessageWebhook(ctx context.Context, title string, message domain.Message) error {
	settings, err := a.store.NotificationSettings().ByOrganization(ctx, message.OrganizationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if !settings.WebhookEnabled || strings.TrimSpace(settings.WebhookURL) == "" {
		return nil
	}
	payload := AgentMessageWebhookPayload{
		Event:            AgentMessageCreatedWebhookEvent,
		Title:            strings.TrimSpace(title),
		OrganizationID:   message.OrganizationID,
		ConversationType: message.ConversationType,
		ConversationID:   message.ConversationID,
		Message:          message,
		CreatedAt:        time.Now().UTC(),
	}
	return a.postWebhook(ctx, settings, payload)
}

func (a *App) postWebhook(ctx context.Context, settings domain.NotificationSettings, payload AgentMessageWebhookPayload) error {
	if payload.Event == "" {
		payload.Event = AgentMessageCreatedWebhookEvent
	}
	if payload.Delivery == "" {
		payload.Delivery = id.New("dlv")
	}
	if payload.CreatedAt.IsZero() {
		payload.CreatedAt = time.Now().UTC()
	}
	webhookURL, err := renderWebhookURL(settings.WebhookURL, payload)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	timeout := a.opts.WebhookTimeout
	if timeout <= 0 {
		timeout = defaultWebhookTimeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AgentX-Event", payload.Event)
	req.Header.Set("X-AgentX-Delivery", payload.Delivery)
	req.Header.Set("X-AgentX-Timestamp", timestamp)
	req.Header.Set("X-AgentX-Signature", signWebhookPayload(settings.WebhookSecret, timestamp, body))

	client := a.opts.WebhookHTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWebhookDeliveryFailed, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: status %d", ErrWebhookDeliveryFailed, resp.StatusCode)
	}
	return nil
}

func signWebhookPayload(secret string, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func validateWebhookURL(value string, enabled bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if enabled {
			return "", ErrInvalidInput
		}
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrInvalidInput
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrInvalidInput
	}
	if parsed.User != nil {
		return "", ErrInvalidInput
	}
	if hasWebhookURLPlaceholder(value) {
		return value, nil
	}
	return parsed.String(), nil
}

func renderWebhookURL(value string, payload AgentMessageWebhookPayload) (string, error) {
	title := strings.TrimSpace(payload.Title)
	if title == "" {
		title = "Agent"
	}
	body := truncateWebhookURLBody(payload.Message.Body)
	rendered := strings.NewReplacer(
		"${title}", escapeWebhookURLValue(title),
		"${body}", escapeWebhookURLValue(body),
		"$%7Btitle%7D", escapeWebhookURLValue(title),
		"$%7Bbody%7D", escapeWebhookURLValue(body),
		"$%7btitle%7d", escapeWebhookURLValue(title),
		"$%7bbody%7d", escapeWebhookURLValue(body),
		"%24%7Btitle%7D", escapeWebhookURLValue(title),
		"%24%7Bbody%7D", escapeWebhookURLValue(body),
		"%24%7btitle%7d", escapeWebhookURLValue(title),
		"%24%7bbody%7d", escapeWebhookURLValue(body),
	).Replace(strings.TrimSpace(value))
	return validateWebhookURL(rendered, true)
}

func hasWebhookURLPlaceholder(value string) bool {
	if strings.Contains(value, "${title}") || strings.Contains(value, "${body}") {
		return true
	}
	lower := strings.ToLower(value)
	return strings.Contains(lower, "$%7btitle%7d") ||
		strings.Contains(lower, "$%7bbody%7d") ||
		strings.Contains(lower, "%24%7btitle%7d") ||
		strings.Contains(lower, "%24%7bbody%7d")
}

func escapeWebhookURLValue(value string) string {
	escaped := url.QueryEscape(value)
	return strings.ReplaceAll(escaped, "+", "%20")
}

func truncateWebhookURLBody(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "New message"
	}
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= webhookURLBodyLimit {
		return value
	}
	return string(runes[:webhookURLBodyLimit-3]) + "..."
}

func defaultNotificationSettings(orgID string) domain.NotificationSettings {
	return domain.NotificationSettings{
		OrganizationID: orgID,
		CreatedAt:      time.Time{},
		UpdatedAt:      time.Time{},
	}
}

func redactNotificationSettings(settings domain.NotificationSettings) domain.NotificationSettings {
	settings.WebhookSecretConfigured = settings.WebhookSecret != ""
	settings.WebhookSecret = ""
	return settings
}
