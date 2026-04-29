package procpool

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"sync"
	"time"
)

var ErrProcessDead = errors.New("procpool: process is dead")

type StartFunc func(ctx context.Context) *exec.Cmd

type Options struct {
	IdleTimeout time.Duration
}

type ProcessPool struct {
	mu        sync.Mutex
	processes map[string]*ManagedProcess
	opts      Options
	ctx       context.Context
	cancel    context.CancelFunc
}

func New(opts Options) *ProcessPool {
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = 30 * time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &ProcessPool{
		processes: make(map[string]*ManagedProcess),
		opts:      opts,
		ctx:       ctx,
		cancel:    cancel,
	}
	go p.idleReaper()
	return p
}

func (p *ProcessPool) GetOrCreate(key string, start StartFunc) (*ManagedProcess, bool, error) {
	p.mu.Lock()
	if mp, ok := p.processes[key]; ok && mp.Alive() {
		p.mu.Unlock()
		return mp, false, nil
	}
	delete(p.processes, key)
	p.mu.Unlock()

	cmd := start(p.ctx)
	mp, err := startProcess(p, key, cmd)
	if err != nil {
		return nil, false, err
	}

	p.mu.Lock()
	if existing, ok := p.processes[key]; ok && existing.Alive() {
		p.mu.Unlock()
		mp.Kill()
		return existing, false, nil
	}
	p.processes[key] = mp
	p.mu.Unlock()

	slog.Info("procpool: process started", "key", key, "pid", mp.cmd.Process.Pid)
	return mp, true, nil
}

func (p *ProcessPool) Get(key string) (*ManagedProcess, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	mp, ok := p.processes[key]
	if ok && !mp.Alive() {
		delete(p.processes, key)
		return nil, false
	}
	return mp, ok
}

func (p *ProcessPool) remove(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.processes, key)
}

func (p *ProcessPool) Kill(key string) {
	p.mu.Lock()
	mp, ok := p.processes[key]
	if ok {
		delete(p.processes, key)
	}
	p.mu.Unlock()
	if ok {
		mp.Kill()
	}
}

func (p *ProcessPool) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	processes := make([]*ManagedProcess, 0, len(p.processes))
	for _, mp := range p.processes {
		processes = append(processes, mp)
	}
	p.processes = make(map[string]*ManagedProcess)
	p.mu.Unlock()

	p.cancel()

	done := make(chan struct{})
	go func() {
		for _, mp := range processes {
			<-mp.done
		}
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *ProcessPool) idleReaper() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.reapIdle()
		}
	}
}

func (p *ProcessPool) reapIdle() {
	now := time.Now()
	p.mu.Lock()
	var toKill []*ManagedProcess
	for key, mp := range p.processes {
		if now.Sub(mp.LastUsedAt()) > p.opts.IdleTimeout {
			toKill = append(toKill, mp)
			delete(p.processes, key)
		}
	}
	p.mu.Unlock()

	for _, mp := range toKill {
		slog.Info("procpool: reaping idle process", "key", mp.Key)
		mp.Kill()
	}
}
