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

// deliverRecheckInterval is how often a blocked dispatch re-checks whether a
// reader is still attached before routing a line to the fallback handler.
const deliverRecheckInterval = 250 * time.Millisecond

// claimRecheckInterval is how often dispatch checks for the reader of a
// claimed turn to attach.
const claimRecheckInterval = 20 * time.Millisecond

// outputDrainDelay bounds how long an exited process's stdout and stderr are
// still read: a grandchild that inherited them can hold them open.
var outputDrainDelay = 2 * time.Second

type ManagedProcess struct {
	Key  string
	cmd  *exec.Cmd
	pool *ProcessPool

	stdin   io.WriteCloser
	stdinMu sync.Mutex
	// stdout and stderr are read by this type rather than through
	// StdoutPipe/StderrPipe, which Wait closes as soon as the process exits —
	// losing the last lines of output, or the stderr explaining the exit.
	stdout     *os.File
	stderr     *os.File
	outputDone chan struct{}

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

	// claimDeadline is set while a turn the process started on its own has been
	// claimed but no reader has adopted it yet; see ClaimTurn.
	claimMu       sync.Mutex
	claimDeadline time.Time

	Mu       sync.Mutex
	UserData map[any]any

	// signature records the launch arguments the process was started with;
	// see ProcessPool.GetOrCreateSigned.
	signature string
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
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		cancel()
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		return nil, err
	}
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	err = cmd.Start()
	// The child has its own copies of the write ends now.
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	if err != nil {
		cancel()
		_ = stdoutReader.Close()
		_ = stderrReader.Close()
		return nil, err
	}

	mp := &ManagedProcess{
		Key:         key,
		cmd:         cmd,
		pool:        pool,
		stdin:       stdin,
		stdout:      stdoutReader,
		stderr:      stderrReader,
		outputDone:  make(chan struct{}),
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

	var output sync.WaitGroup
	output.Add(2)
	go func() {
		defer output.Done()
		mp.readStdout()
	}()
	go func() {
		defer output.Done()
		_, _ = io.Copy(&mp.stderrBuf, stderrReader)
	}()
	go func() {
		output.Wait()
		close(mp.outputDone)
	}()
	go mp.dispatch()
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

// ClaimTurn takes the turn for output the process produced on its own — a turn
// nobody sent a prompt for, such as an agent waking up when its background task
// finishes. It never blocks: it fails when another turn holds the process.
//
// While the claim is pending, dispatch holds lines for the reader that adopts
// it (AttachReader, then AdoptClaim) instead of handing them to the fallback
// handler. A claim nobody adopts within timeout is abandoned.
func (mp *ManagedProcess) ClaimTurn(timeout time.Duration) bool {
	if !mp.Alive() {
		return false
	}
	select {
	case <-mp.turnMu:
	default:
		return false
	}
	mp.turnHeld.Store(true)
	mp.lastUsedAt.Store(time.Now())
	mp.claimMu.Lock()
	mp.claimDeadline = time.Now().Add(timeout)
	mp.claimMu.Unlock()
	return true
}

// AdoptClaim hands a pending claim to the caller, who now holds the turn and
// must release it with ReleaseTurn. Attach the reader first so no held line is
// routed to the fallback in between. It reports false when there is no pending
// claim, e.g. because it was abandoned.
func (mp *ManagedProcess) AdoptClaim() bool {
	mp.claimMu.Lock()
	defer mp.claimMu.Unlock()
	if mp.claimDeadline.IsZero() {
		return false
	}
	mp.claimDeadline = time.Time{}
	return true
}

// AbandonClaim drops a pending claim and releases its turn.
func (mp *ManagedProcess) AbandonClaim() {
	mp.claimMu.Lock()
	pending := !mp.claimDeadline.IsZero()
	mp.claimDeadline = time.Time{}
	mp.claimMu.Unlock()
	if pending {
		mp.ReleaseTurn()
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
		if mp.deliverToClaim(line) {
			continue
		}
		mp.lastUsedAt.Store(time.Now())
		h(line)
		// The handler may have claimed the turn this line starts.
		mp.deliverToClaim(line)
	}
}

// deliverToClaim holds a line for the reader that is about to adopt a pending
// claim, reporting whether it landed. A claim whose reader never shows up is
// abandoned at its deadline and the line goes back to the fallback path.
func (mp *ManagedProcess) deliverToClaim(line []byte) bool {
	for {
		mp.claimMu.Lock()
		deadline := mp.claimDeadline
		readers := mp.readers.Load()
		if !deadline.IsZero() && readers <= 0 && !time.Now().Before(deadline) {
			mp.claimDeadline = time.Time{}
			mp.claimMu.Unlock()
			slog.Warn("procpool: claimed turn was never adopted; releasing it", "key", mp.Key)
			mp.ReleaseTurn()
			return false
		}
		mp.claimMu.Unlock()
		if readers > 0 {
			return mp.deliver(line, false)
		}
		if deadline.IsZero() {
			return false
		}
		timer := time.NewTimer(claimRecheckInterval)
		select {
		case <-timer.C:
		case <-mp.ctx.Done():
			timer.Stop()
			return true
		}
	}
}

// deliver hands a line to the attached reader, reporting whether it landed.
// With no fallback handler installed there is nowhere else for the line to go,
// so it waits for a reader as this type always used to.
//
// While a reader is attached the send also waits indefinitely, even if the
// reader is momentarily not consuming (e.g. a turn blocked on a user's answer
// to AskUserQuestion): dropping the line would silently lose agent output. The
// wait is chunked so a reader that detaches mid-send is noticed and the line
// is rerouted to the fallback instead of stalling dispatch forever.
func (mp *ManagedProcess) deliver(line []byte, waitForReader bool) bool {
	for {
		if !waitForReader && mp.readers.Load() <= 0 {
			return false
		}
		if waitForReader {
			select {
			case mp.stdoutLines <- line:
				return true
			case <-mp.ctx.Done():
				return true
			}
		}
		timer := time.NewTimer(deliverRecheckInterval)
		select {
		case mp.stdoutLines <- line:
			timer.Stop()
			return true
		case <-timer.C:
		case <-mp.ctx.Done():
			timer.Stop()
			return true
		}
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

func (mp *ManagedProcess) readStdout() {
	defer close(mp.rawLines)
	scanner := bufio.NewScanner(mp.stdout)
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
	if err := scanner.Err(); err != nil && mp.ctx.Err() == nil && !errors.Is(err, os.ErrClosed) {
		slog.Warn("procpool: stdout scan error", "key", mp.Key, "error", err)
	}
}

func (mp *ManagedProcess) waitForExit() {
	_ = mp.cmd.Wait()
	// Read what the process wrote before exiting, unless it was killed and
	// nobody wants it any more.
	if mp.ctx.Err() == nil {
		timer := time.NewTimer(outputDrainDelay)
		select {
		case <-mp.outputDone:
		case <-timer.C:
		}
		timer.Stop()
	}
	_ = mp.stdout.Close()
	_ = mp.stderr.Close()
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
