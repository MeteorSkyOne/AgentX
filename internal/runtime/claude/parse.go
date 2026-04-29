package claude

import "github.com/meteorsky/agentx/internal/runtime"

func AssistantContent(payload map[string]any) (string, string, []runtime.ProcessItem) {
	return assistantContent(payload)
}

func ClaudeUsage(payload map[string]any) *runtime.Usage {
	return claudeUsage(payload)
}

func IsErrorResult(payload map[string]any) bool {
	return isErrorResult(payload)
}

func ResultError(payload map[string]any) string {
	return resultError(payload)
}

func IsStaleSessionError(text string) bool {
	return isStaleSessionError(text)
}

func StringValue(values map[string]any, key string) string {
	return stringValue(values, key)
}

func StreamJSONInput(input runtime.Input) ([]byte, error) {
	return claudeStreamJSONInput(input)
}

func HasImageAttachments(input runtime.Input) bool {
	return hasImageAttachments(input)
}
