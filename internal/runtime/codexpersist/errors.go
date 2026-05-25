package codexpersist

import (
	"encoding/json"
	"fmt"
	"strings"
)

const maxCodexErrorDetailBytes = 4000

func codexAppServerExitedError(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return "codex app-server exited"
	}
	return truncateCodexErrorDetail("codex app-server exited: " + stderr)
}

func codexNotificationErrorMessage(msg jsonRPCMessage) string {
	params := notificationParams(msg)
	base := ""
	if msg.Error != nil {
		base = msg.Error.Error()
	} else if value := stringVal(params, "message"); value != "" {
		base = value
	} else if value := nestedStringVal(params, "error", "message"); value != "" {
		base = value
	} else if value := stringVal(params, "error"); value != "" {
		base = value
	} else if value := codexDetailValue(msg.Params); value != "" && value != "null" && len(params) == 0 {
		base = value
	}
	if base == "" {
		base = "codex app-server error"
	}

	detail := codexErrorDetail(params)
	if detail == "" || strings.Contains(base, detail) {
		return truncateCodexErrorDetail(base)
	}
	return truncateCodexErrorDetail(base + ": " + detail)
}

func codexErrorDetail(params map[string]any) string {
	if len(params) == 0 {
		return ""
	}

	var parts []string
	appendField := func(label string, value any) {
		if text := codexDetailValue(value); text != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", label, text))
		}
	}

	appendField("code", params["code"])
	appendField("status", params["status"])
	appendField("reason", params["reason"])
	appendField("detail", params["detail"])
	appendField("details", params["details"])
	appendField("cause", params["cause"])
	appendField("data", params["data"])

	if nested, ok := params["error"].(map[string]any); ok {
		appendField("error_code", nested["code"])
		appendField("error_detail", nested["detail"])
		appendField("error_details", nested["details"])
		appendField("error_data", nested["data"])
	}

	if len(parts) > 0 {
		return strings.Join(parts, ", ")
	}
	if stringVal(params, "message") == "" && stringVal(params, "error") == "" {
		return codexDetailValue(params)
	}
	return ""
}

func codexDetailValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64, bool, int, int64:
		return fmt.Sprint(typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}

func nestedStringVal(m map[string]any, key string, nestedKey string) string {
	nested, _ := m[key].(map[string]any)
	return stringVal(nested, nestedKey)
}

func truncateCodexErrorDetail(text string) string {
	if len(text) <= maxCodexErrorDetailBytes {
		return text
	}
	return text[:maxCodexErrorDetailBytes] + "...(truncated)"
}
