package procpool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// deliverTimeout bounds how long dispatch waits for an attached reader before
// treating a line as unclaimed.
const deliverTimeout = 250 * time.Millisecond

type ManagedProcess struct {
	Key  string
	cmd  *exec.Cmd
	pool *ProcessPool

	stdin   io.WriteCloser
	stdinMu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc

	rawLines    chan []byte
	stdoutLines chan []byte
	readers     atomic.Int32
	fallback    atomic.Value
	done        chan struct{}
	alive       atomic.Bool
	turnHeld    atomic.Bool
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
		rawLines:    make(chan []byte, 64),
		stdoutLines: make(chan []byte, 64),
		done:        make(chan struct{}),
		turnMu:      make(chan struct{}, 1),
	}
	mp.alive.Store(true)
	mp.lastUsedAt.Store(time.Now())
	mp.turnMu <- struct{}{}

	go mp.dispatch()
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
	return mp.write(append(data, '\n'))
}

func (mp *ManagedProcess) WriteBytes(data []byte) error {
	return mp.write(data)
}

func (mp *ManagedProcess) write(data []byte) error {
	if !mp.Alive() {
		return ErrProcessDead
	}

	mp.stdinMu.Lock()
	defer mp.stdinMu.Unlock()

	select {
	case <-mp.done:
		return ErrProcessDead
	default:
	}

	_, err := mp.stdin.Write(data)
	if err != nil && isDeadProcessWriteError(err) {
		return ErrProcessDead
	}
	return err
}

func (mp *ManagedProcess) AcquireTurn(ctx context.Context) error {
	select {
	case <-mp.turnMu:
		if !mp.Alive() {
			mp.ReleaseTurn()
			return ErrProcessDead
		}
		mp.turnHeld.Store(true)
		mp.lastUsedAt.Store(time.Now())
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-mp.done:
		return ErrProcessDead
	}
}

func (mp *ManagedProcess) ReleaseTurn() {
	mp.turnHeld.Store(false)
	mp.lastUsedAt.Store(time.Now())
	select {
	case mp.turnMu <- struct{}{}:
	default:
	}
}

func (mp *ManagedProcess) StdoutLines() <-chan []byte {
	return mp.stdoutLines
}

// AttachReader marks that a caller is consuming StdoutLines. Lines that arrive
// while no reader is attached are handed to the fallback handler instead of
// being buffered for whoever reads next.
func (mp *ManagedProcess) AttachReader() {
	mp.readers.Add(1)
}

func (mp *ManagedProcess) DetachReader() {
	if mp.readers.Add(-1) < 0 {
		mp.readers.Store(0)
	}
}

// SetFallbackHandler installs the handler for lines nobody is reading. It runs
// on the dispatch goroutine, so it must not block.
func (mp *ManagedProcess) SetFallbackHandler(h func(line []byte)) {
	if h == nil {
		mp.fallback.Store((func([]byte))(nil))
		return
	}
	mp.fallback.Store(h)
}

func (mp *ManagedProcess) fallbackHandler() func([]byte) {
	h, _ := mp.fallback.Load().(func([]byte))
	return h
}

// dispatch is the sole producer of stdoutLines. It routes each line to the
// attached reader when there is one, and to the fallback handler otherwise.
func (mp *ManagedProcess) dispatch() {
	defer close(mp.stdoutLines)
	for line := range mp.rawLines {
		h := mp.fallbackHandler()
		if mp.deliver(line, h == nil) {
			continue
		}
		mp.lastUsedAt.Store(time.Now())
		h(line)
	}
}

// deliver hands a line to the attached reader, reporting whether it landed.
// With no fallback handler installed there is nowhere else for the line to go,
// so it waits for a reader as this type always used to.
//
// Otherwise the send is bounded: it covers the race where the reader detaches
// mid-send, and a dropped line is far better than a stalled dispatch goroutine,
// since a stall would stop the fallback from answering control requests.
func (mp *ManagedProcess) deliver(line []byte, waitForReader bool) bool {
	if mp.readers.Load() <= 0 && !waitForReader {
		return false
	}
	var timeout <-chan time.Time
	if !waitForReader {
		timer := time.NewTimer(deliverTimeout)
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case mp.stdoutLines <- line:
		return true
	case <-timeout:
		return false
	case <-mp.ctx.Done():
		return true
	}
}

func (mp *ManagedProcess) Alive() bool {
	return mp.alive.Load()
}

func (mp *ManagedProcess) InUse() bool {
	return mp.turnHeld.Load()
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
	defer close(mp.rawLines)
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
		case mp.rawLines <- copied:
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
	close(mp.done)
	mp.pool.remove(mp)
	slog.Info("procpool: process exited", "key", mp.Key)
}

func isDeadProcessWriteError(err error) bool {
	return errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, os.ErrClosed) ||
		errors.Is(err, syscall.EBADF) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ESRCH)
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
