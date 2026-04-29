package procpool

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func echoStartFunc(ctx context.Context) *exec.Cmd {
	return exec.CommandContext(ctx, "cat")
}

func TestGetOrCreate(t *testing.T) {
	pool := New(Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc1, isNew, err := pool.GetOrCreate("key1", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}
	if !isNew {
		t.Fatal("expected isNew=true for first creation")
	}
	if !proc1.Alive() {
		t.Fatal("expected process to be alive")
	}

	proc2, isNew2, err := pool.GetOrCreate("key1", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}
	if isNew2 {
		t.Fatal("expected isNew=false for existing process")
	}
	if proc1 != proc2 {
		t.Fatal("expected same process instance")
	}
}

func TestGetOrCreateDifferentKeys(t *testing.T) {
	pool := New(Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc1, _, err := pool.GetOrCreate("key1", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}
	proc2, isNew, err := pool.GetOrCreate("key2", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}
	if !isNew {
		t.Fatal("expected isNew=true for different key")
	}
	if proc1 == proc2 {
		t.Fatal("expected different process instances for different keys")
	}
}

func TestKill(t *testing.T) {
	pool := New(Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("key1", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}
	pool.Kill("key1")
	<-proc.Done()
	if proc.Alive() {
		t.Fatal("expected process to be dead after kill")
	}

	_, ok := pool.Get("key1")
	if ok {
		t.Fatal("expected key1 to be removed from pool after kill")
	}
}

func TestTurnCoordination(t *testing.T) {
	pool := New(Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("key1", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := proc.AcquireTurn(ctx); err != nil {
		t.Fatal(err)
	}

	shortCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	err = proc.AcquireTurn(shortCtx)
	if err == nil {
		t.Fatal("expected error acquiring turn while held")
	}

	proc.ReleaseTurn()

	if err := proc.AcquireTurn(ctx); err != nil {
		t.Fatal("expected to acquire turn after release")
	}
	proc.ReleaseTurn()
}

func TestShutdown(t *testing.T) {
	pool := New(Options{IdleTimeout: 1 * time.Hour})

	proc1, _, err := pool.GetOrCreate("key1", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}
	proc2, _, err := pool.GetOrCreate("key2", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}

	<-proc1.Done()
	<-proc2.Done()
	if proc1.Alive() || proc2.Alive() {
		t.Fatal("expected all processes to be dead after shutdown")
	}
}

func TestProcessDeath(t *testing.T) {
	pool := New(Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("key1", func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "true")
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-proc.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process should have exited quickly")
	}

	if proc.Alive() {
		t.Fatal("expected process to be dead")
	}

	_, ok := pool.Get("key1")
	if ok {
		t.Fatal("expected dead process to be removed from pool")
	}

	proc2, isNew, err := pool.GetOrCreate("key1", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}
	if !isNew {
		t.Fatal("expected new process after death")
	}
	if proc2 == proc {
		t.Fatal("expected different process instance")
	}
}

func TestWriteAndRead(t *testing.T) {
	pool := New(Options{IdleTimeout: 1 * time.Hour})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("key1", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}

	if err := proc.WriteJSON(map[string]string{"hello": "world"}); err != nil {
		t.Fatal(err)
	}

	select {
	case line := <-proc.StdoutLines():
		if string(line) != `{"hello":"world"}` {
			t.Fatalf("unexpected line: %s", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for stdout line")
	}
}

func TestIdleReaping(t *testing.T) {
	pool := New(Options{IdleTimeout: 100 * time.Millisecond})
	defer pool.Shutdown(context.Background())

	proc, _, err := pool.GetOrCreate("key1", echoStartFunc)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)
	pool.reapIdle()

	select {
	case <-proc.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("expected process to be reaped")
	}

	_, ok := pool.Get("key1")
	if ok {
		t.Fatal("expected reaped process to be removed from pool")
	}
}
