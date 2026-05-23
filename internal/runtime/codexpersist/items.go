package codexpersist

import (
	"strings"

	"github.com/meteorsky/agentx/internal/runtime"
)

func normalizeItemType(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "-", "_")
	return strings.ToLower(value)
}

func itemToProcessItem(item map[string]any, status string) *runtime.ProcessItem {
	if item == nil {
		return nil
	}
	rawType, _ := item["type"].(string)
	itemType := normalizeItemType(rawType)
	switch itemType {
	case "function_call", "functioncall", "command_execution", "commandexecution",
		"file_change", "filechange", "mcp_tool_call", "mcptoolcall",
		"web_search", "websearch", "file_search", "filesearch",
		"dynamic_tool_call", "dynamictoolcall", "tool_call", "toolcall":
		pi := &runtime.ProcessItem{
			Type:       "tool_call",
			ToolName:   itemToolName(item),
			ToolCallID: stringVal(item, "id"),
			Status:     status,
			Raw:        item,
		}
		if input, ok := item["input"]; ok {
			pi.Input = input
		}
		if output, ok := item["output"]; ok && status == "completed" {
			pi.Output = output
		}
		return pi
	case "collabagenttoolcall":
		return collabAgentToolCallToProcessItem(item, status)
	case "message", "agent_message":
		return nil
	case "reasoning":
		text := stringVal(item, "text")
		if text == "" {
			if summary, ok := item["summary"].([]any); ok && len(summary) > 0 {
				if first, ok := summary[0].(map[string]any); ok {
					text = stringVal(first, "text")
				}
			}
		}
		if text == "" {
			return nil
		}
		return &runtime.ProcessItem{Type: "thinking", Text: text, Raw: item}
	}
	return nil
}

func collabAgentToolCallToProcessItem(item map[string]any, status string) *runtime.ProcessItem {
	tool := normalizeItemType(stringVal(item, "tool"))
	if tool != "spawnagent" {
		return nil
	}
	callID := stringVal(item, "id")
	if status == "completed" {
		itemStatus := normalizeItemType(stringVal(item, "status"))
		if itemStatus == "failed" {
			itemStatus = "error"
		}
		return &runtime.ProcessItem{
			Type:       "tool_result",
			ToolName:   "Agent",
			ToolCallID: callID,
			Status:     itemStatus,
			Raw:        item,
		}
	}
	var input any
	if prompt := stringVal(item, "prompt"); prompt != "" {
		input = map[string]any{"description": prompt}
	}
	return &runtime.ProcessItem{
		Type:       "tool_call",
		ToolName:   "Agent",
		ToolCallID: callID,
		Status:     "started",
		Input:      input,
		Raw:        item,
	}
}

func completedAgentMessageText(item map[string]any) string {
	if item == nil {
		return ""
	}
	itemType := normalizeItemType(stringVal(item, "type"))
	if itemType != "agentmessage" && itemType != "agent_message" && itemType != "message" {
		return ""
	}
	for _, key := range []string{"text", "message"} {
		if text := stringVal(item, key); text != "" {
			return text
		}
	}
	if content, ok := item["content"].([]any); ok {
		var out strings.Builder
		for _, raw := range content {
			part, _ := raw.(map[string]any)
			if part == nil {
				continue
			}
			partType := normalizeItemType(stringVal(part, "type"))
			if partType != "output_text" && partType != "text" {
				continue
			}
			text := stringVal(part, "text")
			if text == "" {
				continue
			}
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(text)
		}
		return out.String()
	}
	return ""
}

func completedPlanText(item map[string]any, state *notificationState) (string, bool) {
	if item == nil {
		return "", false
	}
	if stringVal(item, "type") != "plan" {
		return "", false
	}
	text := stringVal(item, "text")
	itemID := stringVal(item, "id")
	return text, itemID != "" && state.streamedPlanItems[itemID]
}

func itemToolName(item map[string]any) string {
	if name := stringVal(item, "name"); name != "" {
		return name
	}
	if name := stringVal(item, "toolName"); name != "" {
		return name
	}
	return stringVal(item, "type")
}

func stringVal(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}
