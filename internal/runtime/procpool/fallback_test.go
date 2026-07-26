package procpool

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestFallbackReceivesUnclaimedLines(t *testing.T) {
	pool := New(Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("fallback", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}

	got := make(chan string, 4)
	proc.SetFallbackHandler(func(line []byte) {
		got <- string(line)
	})

	if err := proc.WriteBytes([]byte("orphan line\n")); err != nil {
		t.Fatal(err)
	}

	select {
	case line := <-got:
		if line != "orphan line" {
			t.Fatalf("expected %q, got %q", "orphan line", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("fallback handler never saw the unclaimed line")
	}
}

func TestAttachedReaderTakesPrecedenceOverFallback(t *testing.T) {
	pool := New(Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("attached", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}

	fellBack := make(chan string, 4)
	proc.SetFallbackHandler(func(line []byte) {
		fellBack <- string(line)
	})

	proc.AttachReader()
	defer proc.DetachReader()

	if err := proc.WriteBytes([]byte("claimed line\n")); err != nil {
		t.Fatal(err)
	}

	select {
	case line := <-proc.StdoutLines():
		if string(line) != "claimed line" {
			t.Fatalf("expected %q, got %q", "claimed line", string(line))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("attached reader never received the line")
	}

	select {
	case line := <-fellBack:
		t.Fatalf("fallback should not run while a reader is attached, got %q", line)
	case <-time.After(200 * time.Millisecond):
	}
}

// Without a fallback handler there is nowhere else for a line to go, so it must
// wait for a reader rather than being dropped.
func TestLinesWaitForReaderWhenNoFallbackInstalled(t *testing.T) {
	pool := New(Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("buffered", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}

	if err := proc.WriteBytes([]byte("early line\n")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	proc.AttachReader()
	defer proc.DetachReader()

	select {
	case line := <-proc.StdoutLines():
		if string(line) != "early line" {
			t.Fatalf("expected %q, got %q", "early line", string(line))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("line emitted before a reader attached was lost")
	}
}

// A reader that is attached but momentarily not consuming — a turn blocked on
// a user's answer — must not lose lines to the fallback handler, even once the
// channel buffers fill up.
func TestSlowAttachedReaderDoesNotLoseLinesToFallback(t *testing.T) {
	pool := New(Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("slow-reader", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}

	fellBack := make(chan string, 256)
	proc.SetFallbackHandler(func(line []byte) {
		fellBack <- string(line)
	})

	proc.AttachReader()
	defer proc.DetachReader()

	// More lines than the raw + stdout channel buffers combined, so dispatch
	// has to block on a full channel while the reader stays idle.
	const total = 200
	for i := 0; i < total; i++ {
		if err := proc.WriteBytes([]byte(fmt.Sprintf("line-%d\n", i))); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(600 * time.Millisecond)

	deadline := time.After(5 * time.Second)
	for received := 0; received < total; received++ {
		select {
		case <-proc.StdoutLines():
		case <-deadline:
			t.Fatalf("received only %d of %d lines before timing out", received, total)
		}
	}

	select {
	case line := <-fellBack:
		t.Fatalf("line diverted to fallback while a reader was attached: %q", line)
	default:
	}
}

func TestDetachReaderDoesNotGoNegative(t *testing.T) {
	pool := New(Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("counter", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}

	proc.DetachReader()
	proc.DetachReader()
	if n := proc.readers.Load(); n != 0 {
		t.Fatalf("expected reader count clamped to 0, got %d", n)
	}

	proc.AttachReader()
	if n := proc.readers.Load(); n != 1 {
		t.Fatalf("expected reader count 1 after attach, got %d", n)
	}
}
