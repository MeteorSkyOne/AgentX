package httpapi

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/meteorsky/agentx/internal/domain"
)

type messageProcessItemDetail struct {
	Item   map[string]any `json:"item"`
	Result map[string]any `json:"result,omitempty"`
}

func redactMessagesProcessDetails(messages []domain.Message) []domain.Message {
	redacted := make([]domain.Message, len(messages))
	for i, message := range messages {
		redacted[i] = redactMessageProcessDetails(message)
	}
	return redacted
}

func redactMessageProcessDetails(message domain.Message) domain.Message {
	if len(message.Metadata) == 0 {
		return message
	}
	metadata := copyMetadata(message.Metadata)
	items, ok := processItemsFromMetadata(metadata)
	if !ok {
		message.Metadata = metadata
		return message
	}

	summary := make([]map[string]any, 0, len(items))
	for index, item := range items {
		summary = append(summary, processItemSummary(item, index))
	}
	metadata["process"] = summary
	message.Metadata = metadata
	return message
}

func redactEventProcessDetails(event domain.Event) domain.Event {
	switch payload := event.Payload.(type) {
	case domain.MessageCreatedPayload:
		payload.Message = redactMessageProcessDetails(payload.Message)
		event.Payload = payload
	case domain.MessageUpdatedPayload:
		payload.Message = redactMessageProcessDetails(payload.Message)
		event.Payload = payload
	case domain.MessageHistoryChunkPayload:
		payload.Messages = redactMessagesProcessDetails(payload.Messages)
		event.Payload = payload
	}
	return event
}

func messageProcessDetail(message domain.Message, index int) (messageProcessItemDetail, bool) {
	items, ok := processItemsFromMetadata(message.Metadata)
	if !ok || index < 0 || index >= len(items) {
		return messageProcessItemDetail{}, false
	}

	item := copyProcessItem(items[index])
	item["process_index"] = index
	result := matchingToolResult(items, index, item)
	if result != nil {
		return messageProcessItemDetail{Item: item, Result: result}, true
	}
	return messageProcessItemDetail{Item: item}, true
}

func parseProcessIndex(value string) (int, bool) {
	index, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || index < 0 {
		return 0, false
	}
	return index, true
}

func processItemsFromMetadata(metadata map[string]any) ([]map[string]any, bool) {
	if len(metadata) == 0 {
		return nil, false
	}
	raw, ok := metadata["process"]
	if !ok || raw == nil {
		return nil, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil || len(items) == 0 {
		return nil, false
	}
	return items, true
}

func processItemSummary(item map[string]any, index int) map[string]any {
	summary := map[string]any{
		"type":          stringValue(item, "type"),
		"process_index": index,
	}
	copyStringField(summary, item, "tool_name")
	copyStringField(summary, item, "tool_call_id")
	copyStringField(summary, item, "status")
	copyStringField(summary, item, "created_at")

	if stringValue(item, "type") == "thinking" {
		copyStringField(summary, item, "text")
		return summary
	}
	summary["has_detail"] = true
	return summary
}

func matchingToolResult(items []map[string]any, index int, item map[string]any) map[string]any {
	if stringValue(item, "type") != "tool_call" {
		return nil
	}
	callID := stringValue(item, "tool_call_id")
	toolName := stringValue(item, "tool_name")
	for i := index + 1; i < len(items); i++ {
		candidate := items[i]
		if stringValue(candidate, "type") != "tool_result" {
			continue
		}
		if callID != "" {
			if stringValue(candidate, "tool_call_id") != callID {
				continue
			}
		} else if candidateTool := stringValue(candidate, "tool_name"); toolName != "" && candidateTool != "" && candidateTool != toolName {
			continue
		}
		result := copyProcessItem(candidate)
		result["process_index"] = i
		return result
	}
	return nil
}

func copyMetadata(metadata map[string]any) map[string]any {
	copy := make(map[string]any, len(metadata))
	for key, value := range metadata {
		copy[key] = value
	}
	return copy
}

func copyProcessItem(item map[string]any) map[string]any {
	copy := make(map[string]any, len(item)+1)
	for key, value := range item {
		copy[key] = value
	}
	return copy
}

func copyStringField(dst map[string]any, src map[string]any, key string) {
	if value := stringValue(src, key); value != "" {
		dst[key] = value
	}
}

func stringValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}
