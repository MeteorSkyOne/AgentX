package cli

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	agentruntime "github.com/meteorsky/agentx/internal/runtime"
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

func TestCloseDoesNotEmitFailureForContextStoppedCommand(t *testing.T) {
	session := NewSession("session_test", func(input agentruntime.Input) (Command, error) {
		return Command{
			Name: os.Args[0],
			Args: []string{"-test.run=TestHelperProcessSleep"},
			Env:  map[string]string{"AGENTX_CLI_HELPER_SLEEP": "1"},
		}, nil
	}, testLineHandler{})

	if err := session.Send(context.Background(), agentruntime.Input{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.Close(ctx); err != nil {
		t.Fatal(err)
	}

	for evt := range session.Events() {
		if evt.Type == agentruntime.EventFailed {
			t.Fatalf("unexpected failed event: %#v", evt)
		}
	}
}

func TestHelperProcessSleep(t *testing.T) {
	if os.Getenv("AGENTX_CLI_HELPER_SLEEP") != "1" {
		return
	}
	time.Sleep(10 * time.Second)
	os.Exit(0)
}

type testLineHandler struct{}

func (testLineHandler) HandleLine(line []byte) ([]agentruntime.Event, error) {
	return nil, nil
}

func (testLineHandler) Finish(stderr string, waitErr error) (agentruntime.Event, bool) {
	if waitErr != nil {
		return agentruntime.Event{Type: agentruntime.EventFailed, Error: waitErr.Error()}, true
	}
	return agentruntime.Event{Type: agentruntime.EventCompleted}, true
}

func (testLineHandler) CurrentSessionID() string {
	return ""
}
