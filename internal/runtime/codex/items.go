package codex

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/meteorsky/agentx/internal/runtime"
)

func stringValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func firstText(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if text := stringValue(values, key); text != "" {
			return text
		}
	}
	return ""
}

func reasoningSummaryText(item map[string]any) string {
	arr, _ := item["summary"].([]any)
	var out strings.Builder
	for _, elem := range arr {
		if text, ok := elem.(string); ok {
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(text)
			continue
		}
		part, _ := elem.(map[string]any)
		if stringValue(part, "type") != "summary_text" {
			continue
		}
		text := stringValue(part, "text")
		if text == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(text)
	}
	if out.Len() > 0 {
		return out.String()
	}
	return stringValue(item, "text")
}

func codexMessageText(item map[string]any) string {
	content, _ := item["content"].([]any)
	var out strings.Builder
	for _, elem := range content {
		part, _ := elem.(map[string]any)
		switch normalizedType(stringValue(part, "type")) {
		case "output_text", "text":
			text := stringValue(part, "text")
			if text == "" {
				continue
			}
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(text)
		}
	}
	if out.Len() > 0 {
		return out.String()
	}
	return firstText(item, "text", "message")
}

func codexStartedToolProcessItems(item map[string]any) []runtime.ProcessItem {
	itemType := normalizedType(stringValue(item, "type"))
	switch itemType {
	case "command_execution", "commandexecution", "function_call", "functioncall", "web_search", "websearch", "file_search", "filesearch", "mcp_tool_call", "mcptoolcall", "dynamic_tool_call", "dynamictoolcall", "file_change", "filechange":
		return []runtime.ProcessItem{codexToolCallItem(item)}
	default:
		return nil
	}
}

func codexToolProcessItems(item map[string]any) []runtime.ProcessItem {
	itemType := normalizedType(stringValue(item, "type"))
	if isCodexToolResultType(itemType) {
		return []runtime.ProcessItem{codexToolResultItem(item)}
	}
	if !isCodexToolCallType(itemType) {
		return nil
	}

	call := codexToolCallItem(item)
	items := []runtime.ProcessItem{call}
	if output, ok := firstPresent(item, "output", "result", "content", "contentItems", "aggregated_output", "aggregatedOutput", "formatted_output", "formattedOutput", "stdout", "stderr"); ok {
		result := codexToolResultItem(item)
		result.Output = output
		items = append(items, result)
	}
	return items
}

func codexToolCallItem(item map[string]any) runtime.ProcessItem {
	return runtime.ProcessItem{
		Type:       "tool_call",
		Text:       codexToolDisplayText(item),
		ToolName:   codexToolName(item),
		ToolCallID: codexToolCallID(item),
		Status:     stringValue(item, "status"),
		Input:      codexToolInput(item),
		Raw:        item,
	}
}

func codexToolResultItem(item map[string]any) runtime.ProcessItem {
	output, _ := firstPresent(item, "output", "result", "content", "contentItems", "aggregated_output", "aggregatedOutput", "formatted_output", "formattedOutput", "stdout", "stderr")
	return runtime.ProcessItem{
		Type:       "tool_result",
		Text:       codexToolDisplayText(item),
		ToolName:   codexToolName(item),
		ToolCallID: codexToolCallID(item),
		Status:     codexToolResultStatus(item),
		Input:      codexToolInput(item),
		Output:     output,
		Raw:        item,
	}
}

func codexCollabSpawnStarted(item map[string]any) []runtime.ProcessItem {
	callID := stringValue(item, "call_id")
	if callID == "" {
		return nil
	}
	var input any
	if prompt := stringValue(item, "prompt"); prompt != "" {
		input = map[string]any{"description": prompt}
	}
	return []runtime.ProcessItem{{
		Type:       "tool_call",
		ToolName:   "Agent",
		ToolCallID: callID,
		Status:     "started",
		Input:      input,
		Raw:        item,
	}}
}

func codexCollabSpawnCompleted(item map[string]any) []runtime.ProcessItem {
	callID := stringValue(item, "call_id")
	if callID == "" {
		return nil
	}
	status := normalizedType(stringValue(item, "status"))
	if strings.Contains(status, "error") || status == "not_found" || status == "notfound" || status == "failed" {
		status = "error"
	}
	return []runtime.ProcessItem{{
		Type:       "tool_result",
		ToolName:   "Agent",
		ToolCallID: callID,
		Status:     status,
		Raw:        item,
	}}
}

func isCodexToolCallType(itemType string) bool {
	switch itemType {
	case "command_execution", "commandexecution", "dynamic_tool_call", "dynamictoolcall", "file_change", "filechange", "web_search", "websearch", "file_search", "filesearch", "mcptoolcall", "mcp_tool_call", "mcp_tool_call_begin", "collabtoolcall", "collab_tool_call", "collab_tool_call_begin", "toolcall", "tool_call", "tool_call_begin", "functioncall", "function_call", "exec_command_begin", "patch_apply_begin":
		return true
	default:
		return false
	}
}

func isCodexToolResultType(itemType string) bool {
	switch itemType {
	case "command_execution", "commandexecution", "dynamic_tool_call", "dynamictoolcall", "file_change", "filechange", "web_search", "websearch", "file_search", "filesearch", "mcptoolresult", "mcp_tool_result", "mcp_tool_call_end", "mcp_tool_call_output", "collabtoolresult", "collab_tool_result", "collab_tool_call_end", "toolresult", "tool_result", "tool_call_end", "toolcalloutput", "tool_call_output", "functioncalloutput", "function_call_output", "custom_tool_call_output", "exec_command_end", "patch_apply_end":
		return true
	default:
		return false
	}
}

func normalizedType(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "-", "_")
	return strings.ToLower(value)
}

func codexToolName(item map[string]any) string {
	switch normalizedType(stringValue(item, "type")) {
	case "command_execution", "commandexecution", "exec_command_begin", "exec_command_end":
		return "Bash"
	case "web_search", "websearch":
		return "WebSearch"
	case "file_search", "filesearch":
		return "FileSearch"
	case "file_change", "filechange", "patch_apply_begin", "patch_apply_end":
		return "Patch"
	case "mcp_tool_call", "mcptoolcall", "mcp_tool_call_begin", "mcp_tool_call_end", "mcp_tool_call_output":
		if tool := stringValue(item, "tool"); tool != "" {
			if server := stringValue(item, "server"); server != "" {
				return server + "." + tool
			}
			return tool
		}
		if name := stringValue(item, "name"); name != "" {
			return name
		}
		return "MCP"
	case "dynamic_tool_call", "dynamictoolcall":
		if tool := stringValue(item, "tool"); tool != "" {
			return tool
		}
		return "Tool"
	}
	switch normalizedType(stringValue(item, "type")) {
	case "exec_command_begin", "exec_command_end":
		return "exec_command"
	case "patch_apply_begin", "patch_apply_end":
		return "apply_patch"
	}
	for _, key := range []string{"tool_name", "toolName", "name", "tool", "command"} {
		value, ok := item[key]
		if !ok || value == nil {
			continue
		}
		if nested, ok := value.(map[string]any); ok {
			if name := stringValue(nested, "name"); name != "" {
				return name
			}
			continue
		}
		if text := stringValue(item, key); text != "" {
			if server := stringValue(item, "server"); server != "" && key == "tool" {
				return server + "." + text
			}
			return text
		}
	}
	return ""
}

func codexToolCallID(item map[string]any) string {
	for _, key := range []string{"tool_call_id", "toolCallId", "call_id", "callId", "id", "item_id", "itemId"} {
		if text := stringValue(item, key); text != "" {
			return text
		}
	}
	return ""
}

func codexToolInput(item map[string]any) any {
	if input, ok := firstPresent(item, "input", "arguments", "args", "params", "parameters", "changes"); ok {
		return decodeJSONValue(input)
	}
	if command, ok := firstPresent(item, "command", "cmd"); ok {
		values := map[string]any{"command": command}
		if cwd := stringValue(item, "cwd"); cwd != "" {
			values["cwd"] = cwd
		}
		return values
	}
	if query, ok := firstPresent(item, "query"); ok {
		return map[string]any{"query": query}
	}
	if action, ok := item["action"].(map[string]any); ok {
		return action
	}
	return nil
}

func codexToolResultStatus(item map[string]any) string {
	if status := stringValue(item, "status"); status != "" {
		return status
	}
	if isError, ok := item["is_error"].(bool); ok && isError {
		return "error"
	}
	if isError, ok := item["error"].(bool); ok && isError {
		return "error"
	}
	return ""
}

func codexToolDisplayText(item map[string]any) string {
	for _, key := range []string{"text", "summary", "message", "formatted_output", "formattedOutput"} {
		if text := stringValue(item, key); text != "" {
			return text
		}
	}
	return ""
}

func firstPresent(values map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		value, ok := values[key]
		if ok && value != nil {
			return value, true
		}
	}
	return nil, false
}

func decodeJSONValue(value any) any {
	text, ok := value.(string)
	if !ok {
		return value
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return value
	}
	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return value
	}
	return decoded
}

func errorText(payload map[string]any) string {
	for _, key := range []string{"message", "error"} {
		if text := stringValue(payload, key); text != "" {
			return text
		}
	}
	return "codex runtime failed"
}
