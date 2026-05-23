package codex

import "strings"

func commandError(stderr string, waitErr error) string {
	if strings.TrimSpace(stderr) != "" {
		return stderr
	}
	if waitErr != nil {
		return waitErr.Error()
	}
	return "codex runtime failed"
}

func mergeMaps(first map[string]string, second map[string]string) map[string]string {
	if len(first) == 0 && len(second) == 0 {
		return nil
	}
	merged := make(map[string]string, len(first)+len(second))
	for key, value := range first {
		merged[key] = value
	}
	for key, value := range second {
		merged[key] = value
	}
	return merged
}
