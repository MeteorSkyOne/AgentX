package cli

import (
	"reflect"
	"testing"
)

func TestSafeCommandArgsRedactsSensitiveArgs(t *testing.T) {
	got := safeCommandArgs([]string{
		"--append-system-prompt",
		"system prompt",
		"-c",
		"developer_instructions=private instructions",
		"user prompt",
	})
	want := []string{
		"--append-system-prompt",
		"<system-prompt:13 chars>",
		"-c",
		"developer_instructions=<value:20 chars>",
		"<prompt:11 chars>",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("safeCommandArgs() = %#v, want %#v", got, want)
	}
}
