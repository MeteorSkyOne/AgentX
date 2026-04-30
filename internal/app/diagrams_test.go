package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestD2RenderCachesResults(t *testing.T) {
	countPath := filepath.Join(t.TempDir(), "count")
	t.Setenv("D2_COUNT_FILE", countPath)
	renderer := newD2Renderer(D2RenderOptions{
		Command: writeD2TestScript(t, `#!/bin/sh
count_file="$D2_COUNT_FILE"
count=$(cat "$count_file" 2>/dev/null || echo 0)
echo $((count + 1)) > "$count_file"
cat >/dev/null
printf '<svg xmlns="http://www.w3.org/2000/svg"><text>ok</text></svg>'
`),
		CacheTTL: time.Hour,
	})

	first, err := renderer.Render(context.Background(), "x -> y")
	if err != nil {
		t.Fatal(err)
	}
	if first.Cached {
		t.Fatal("first render Cached = true, want false")
	}
	second, err := renderer.Render(context.Background(), "x -> y")
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached {
		t.Fatal("second render Cached = false, want true")
	}
	if first.SVG != second.SVG {
		t.Fatalf("cached SVG changed")
	}
	if got := strings.TrimSpace(readTestFile(t, countPath)); got != "1" {
		t.Fatalf("render count = %q, want 1", got)
	}
}

func TestD2RenderConcurrentCallsShareProcess(t *testing.T) {
	countPath := filepath.Join(t.TempDir(), "count")
	t.Setenv("D2_COUNT_FILE", countPath)
	renderer := newD2Renderer(D2RenderOptions{
		Command: writeD2TestScript(t, `#!/bin/sh
count_file="$D2_COUNT_FILE"
count=$(cat "$count_file" 2>/dev/null || echo 0)
echo $((count + 1)) > "$count_file"
cat >/dev/null
sleep 0.1
printf '<svg xmlns="http://www.w3.org/2000/svg"><text>ok</text></svg>'
`),
		CacheTTL: time.Hour,
	})

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := renderer.Render(context.Background(), "shared -> graph")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.TrimSpace(readTestFile(t, countPath)); got != "1" {
		t.Fatalf("render count = %q, want 1", got)
	}
}

func TestD2RenderRejectsLargeInput(t *testing.T) {
	renderer := newD2Renderer(D2RenderOptions{MaxSourceBytes: 8})
	_, err := renderer.Render(context.Background(), strings.Repeat("x", 9))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func TestD2RenderRejectsLargeOutput(t *testing.T) {
	renderer := newD2Renderer(D2RenderOptions{
		Command: writeD2TestScript(t, `#!/bin/sh
cat >/dev/null
printf '<svg xmlns="http://www.w3.org/2000/svg">'
printf 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
printf '</svg>'
`),
		MaxSVGBytes: 16,
	})
	_, err := renderer.Render(context.Background(), "x -> y")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func TestD2RenderReportsMissingCommand(t *testing.T) {
	renderer := newD2Renderer(D2RenderOptions{
		Command: filepath.Join(t.TempDir(), "missing-d2"),
	})
	_, err := renderer.Render(context.Background(), "x -> y")
	if !errors.Is(err, ErrD2CommandUnavailable) {
		t.Fatalf("error = %v, want ErrD2CommandUnavailable", err)
	}
}

func TestD2RenderReportsNonZeroExit(t *testing.T) {
	renderer := newD2Renderer(D2RenderOptions{
		Command: writeD2TestScript(t, `#!/bin/sh
cat >/dev/null
echo "bad diagram" >&2
exit 2
`),
	})
	_, err := renderer.Render(context.Background(), "x ->")
	if !errors.Is(err, ErrD2RenderFailed) {
		t.Fatalf("error = %v, want ErrD2RenderFailed", err)
	}
	if got := D2RenderErrorMessage(err); !strings.Contains(got, "bad diagram") {
		t.Fatalf("D2RenderErrorMessage = %q, want stderr", got)
	}
}

func TestD2RenderReportsTimeout(t *testing.T) {
	renderer := newD2Renderer(D2RenderOptions{
		Command: writeD2TestScript(t, `#!/bin/sh
cat >/dev/null
sleep 1
printf '<svg xmlns="http://www.w3.org/2000/svg"></svg>'
`),
		Timeout: 20 * time.Millisecond,
	})
	_, err := renderer.Render(context.Background(), "x -> y")
	if !errors.Is(err, ErrD2RenderTimeout) {
		t.Fatalf("error = %v, want ErrD2RenderTimeout", err)
	}
}

func writeD2TestScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "d2")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
