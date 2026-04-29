package procpool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

type ManagedProcess struct {
	Key  string
	cmd  *exec.Cmd
	pool *ProcessPool

	stdin   io.WriteCloser
	stdinMu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc

	stdoutLines chan []byte
	done        chan struct{}
	alive       atomic.Bool
	lastUsedAt  atomic.Value

	turnMu chan struct{}

	stderrBuf lockedBuffer

	Mu       sync.Mutex
	UserData map[any]any
}

func startProcess(pool *ProcessPool, key string, cmd *exec.Cmd) (*ManagedProcess, error) {
	ctx, cancel := context.WithCancel(pool.ctx)
	cmd.Cancel = func() error {
		return cmd.Process.Kill()
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	mp := &ManagedProcess{
		Key:         key,
		cmd:         cmd,
		pool:        pool,
		stdin:       stdin,
		ctx:         ctx,
		cancel:      cancel,
		stdoutLines: make(chan []byte, 64),
		done:        make(chan struct{}),
		turnMu:      make(chan struct{}, 1),
	}
	mp.alive.Store(true)
	mp.lastUsedAt.Store(time.Now())
	mp.turnMu <- struct{}{}

	go mp.readStdout(stdout)
	go mp.readStderr(stderr)
	go mp.waitForExit()

	return mp, nil
}

func (mp *ManagedProcess) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	mp.stdinMu.Lock()
	defer mp.stdinMu.Unlock()
	_, err = mp.stdin.Write(append(data, '\n'))
	return err
}

func (mp *ManagedProcess) WriteBytes(data []byte) error {
	mp.stdinMu.Lock()
	defer mp.stdinMu.Unlock()
	_, err := mp.stdin.Write(data)
	return err
}

func (mp *ManagedProcess) AcquireTurn(ctx context.Context) error {
	select {
	case <-mp.turnMu:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-mp.done:
		return ErrProcessDead
	}
}

func (mp *ManagedProcess) ReleaseTurn() {
	mp.lastUsedAt.Store(time.Now())
	select {
	case mp.turnMu <- struct{}{}:
	default:
	}
}

func (mp *ManagedProcess) StdoutLines() <-chan []byte {
	return mp.stdoutLines
}

func (mp *ManagedProcess) Alive() bool {
	return mp.alive.Load()
}

func (mp *ManagedProcess) Done() <-chan struct{} {
	return mp.done
}

func (mp *ManagedProcess) Stderr() string {
	return mp.stderrBuf.String()
}

func (mp *ManagedProcess) Kill() {
	mp.stdinMu.Lock()
	_ = mp.stdin.Close()
	mp.stdinMu.Unlock()
	mp.cancel()
	<-mp.done
}

func (mp *ManagedProcess) LastUsedAt() time.Time {
	if t, ok := mp.lastUsedAt.Load().(time.Time); ok {
		return t
	}
	return time.Time{}
}

func (mp *ManagedProcess) readStdout(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		copied := make([]byte, len(line))
		copy(copied, line)
		select {
		case mp.stdoutLines <- copied:
		case <-mp.ctx.Done():
			return
		}
	}
	if err := scanner.Err(); err != nil && mp.ctx.Err() == nil {
		slog.Warn("procpool: stdout scan error", "key", mp.Key, "error", err)
	}
}

func (mp *ManagedProcess) readStderr(r io.Reader) {
	_, _ = io.Copy(&mp.stderrBuf, r)
}

func (mp *ManagedProcess) waitForExit() {
	_ = mp.cmd.Wait()
	mp.alive.Store(false)
	close(mp.stdoutLines)
	close(mp.done)
	mp.pool.remove(mp.Key)
	slog.Info("procpool: process exited", "key", mp.Key)
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
