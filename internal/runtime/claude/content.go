package claude

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/meteorsky/agentx/internal/runtime"
)

func assistantText(payload map[string]any) string {
	message, _ := payload["message"].(map[string]any)
	content, _ := message["content"].([]any)
	var out strings.Builder
	for _, item := range content {
		part, _ := item.(map[string]any)
		if stringValue(part, "type") != "text" {
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
	return out.String()
}

func assistantThinking(payload map[string]any) string {
	message, _ := payload["message"].(map[string]any)
	content, _ := message["content"].([]any)
	var out strings.Builder
	for _, item := range content {
		part, _ := item.(map[string]any)
		if stringValue(part, "type") != "thinking" {
			continue
		}
		text := stringValue(part, "thinking")
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

func assistantContent(payload map[string]any) (string, string, []runtime.ProcessItem) {
	message, _ := payload["message"].(map[string]any)
	content, _ := message["content"].([]any)
	var textOut strings.Builder
	var thinkingOut strings.Builder
	var process []runtime.ProcessItem
	for _, item := range content {
		part, _ := item.(map[string]any)
		switch stringValue(part, "type") {
		case "text":
			text := stringValue(part, "text")
			if text == "" {
				continue
			}
			if textOut.Len() > 0 {
				textOut.WriteByte('\n')
			}
			textOut.WriteString(text)
		case "thinking":
			text := claudeThinkingText(part)
			if text == "" {
				continue
			}
			if thinkingOut.Len() > 0 {
				thinkingOut.WriteByte('\n')
			}
			thinkingOut.WriteString(text)
			process = append(process, runtime.ProcessItem{
				Type: "thinking",
				Text: text,
				Raw:  part,
			})
		case "tool_use":
			process = append(process, runtime.ProcessItem{
				Type:       "tool_call",
				Text:       stringValue(part, "text"),
				ToolName:   stringValue(part, "name"),
				ToolCallID: stringValue(part, "id"),
				Status:     stringValue(part, "status"),
				Input:      valueOrNil(part, "input"),
				Raw:        part,
			})
		case "tool_result":
			process = append(process, runtime.ProcessItem{
				Type:       "tool_result",
				Text:       stringValue(part, "text"),
				ToolCallID: claudeToolResultCallID(part),
				Status:     claudeToolResultStatus(part),
				Output:     claudeToolResultOutput(part),
				Raw:        part,
			})
		}
	}
	return textOut.String(), thinkingOut.String(), process
}

func claudeThinkingText(part map[string]any) string {
	if text := stringValue(part, "thinking"); text != "" {
		return text
	}
	return stringValue(part, "text")
}

func claudeToolResultCallID(part map[string]any) string {
	for _, key := range []string{"tool_use_id", "tool_call_id", "id"} {
		if text := stringValue(part, key); text != "" {
			return text
		}
	}
	return ""
}

func claudeToolResultStatus(part map[string]any) string {
	if isError, ok := part["is_error"].(bool); ok && isError {
		return "error"
	}
	return stringValue(part, "status")
}

func claudeToolResultOutput(part map[string]any) any {
	if output, ok := firstPresent(part, "content", "result", "output"); ok {
		return output
	}
	return nil
}

func isErrorResult(payload map[string]any) bool {
	if isError, ok := payload["is_error"].(bool); ok && isError {
		return true
	}
	subtype := stringValue(payload, "subtype")
	return subtype != "" && subtype != "success"
}

func resultError(payload map[string]any) string {
	var base string
	for _, key := range []string{"error", "message", "result"} {
		if text := stringValue(payload, key); text != "" {
			base = text
			break
		}
	}
	if base == "" {
		if subtype := stringValue(payload, "subtype"); subtype != "" {
			if raw, err := json.Marshal(payload); err == nil {
				return subtype + ": " + string(raw)
			}
			return subtype
		}
		if raw, err := json.Marshal(payload); err == nil {
			return "claude runtime failed: " + string(raw)
		}
		return "claude runtime failed"
	}

	detail := resultErrorDetail(payload)
	if detail == "" || strings.Contains(base, detail) {
		return base
	}
	return truncateErrorDetail(base + ": " + detail)
}

const maxErrorDetailBytes = 4000

func resultErrorDetail(payload map[string]any) string {
	var parts []string
	appendField := func(label string, value any) {
		if text := detailValue(value); text != "" {
			parts = append(parts, label+"="+text)
		}
	}

	appendField("code", payload["code"])
	appendField("status", payload["status"])
	appendField("reason", payload["reason"])
	appendField("detail", payload["detail"])
	appendField("details", payload["details"])
	appendField("cause", payload["cause"])
	appendField("data", payload["data"])

	if nested, ok := payload["error_details"].(map[string]any); ok {
		appendField("error_code", nested["code"])
		appendField("error_detail", nested["detail"])
		appendField("error_data", nested["data"])
	}

	if len(parts) > 0 {
		return strings.Join(parts, ", ")
	}
	return ""
}

func detailValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		s := strings.TrimSpace(typed)
		if s == "" {
			return ""
		}
		return s
	case json.Number:
		return typed.String()
	case float64, bool, int, int64:
		return fmt.Sprint(typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		s := string(data)
		if s == "null" || s == "{}" || s == "[]" {
			return ""
		}
		return s
	}
}

func truncateErrorDetail(text string) string {
	if len(text) <= maxErrorDetailBytes {
		return text
	}
	return text[:maxErrorDetailBytes] + "...(truncated)"
}

func isAskUserQuestion(part map[string]any) bool {
	return stringValue(part, "type") == "tool_use" && stringValue(part, "name") == "AskUserQuestion"
}

func parseAskUserQuestion(part map[string]any) (string, []runtime.InputRequestOption, string) {
	toolCallID := stringValue(part, "id")
	input, _ := part["input"].(map[string]any)
	if input == nil {
		return "", nil, toolCallID
	}

	// Claude Code uses a questions array: input.questions[0].question / .options
	if questions, ok := input["questions"].([]any); ok && len(questions) > 0 {
		q, _ := questions[0].(map[string]any)
		if q != nil {
			question := stringValue(q, "question")
			options := parseInputOptions(q)
			return question, options, toolCallID
		}
	}

	// Fallback: flat format (input.question / input.options)
	question := stringValue(input, "question")
	options := parseInputOptions(input)
	return question, options, toolCallID
}

func parseInputOptions(src map[string]any) []runtime.InputRequestOption {
	rawOptions, _ := src["options"].([]any)
	if len(rawOptions) == 0 {
		return nil
	}
	options := make([]runtime.InputRequestOption, 0, len(rawOptions))
	for _, opt := range rawOptions {
		optMap, _ := opt.(map[string]any)
		if optMap == nil {
			continue
		}
		options = append(options, runtime.InputRequestOption{
			Label:       stringValue(optMap, "label"),
			Description: stringValue(optMap, "description"),
		})
	}
	return options
}
