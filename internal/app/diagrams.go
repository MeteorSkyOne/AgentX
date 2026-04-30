package app

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	defaultD2Command         = "d2"
	defaultD2Timeout         = 10 * time.Second
	defaultD2CacheTTL        = 24 * time.Hour
	defaultD2CacheMaxEntries = 256
	defaultD2MaxSourceBytes  = 128 * 1024
	defaultD2MaxSVGBytes     = 2 * 1024 * 1024
	maxD2StderrBytes         = 64 * 1024
)

var (
	ErrD2CommandUnavailable = errors.New("D2 command unavailable")
	ErrD2RenderFailed       = errors.New("D2 render failed")
	ErrD2RenderTimeout      = errors.New("D2 render timed out")

	errD2OutputTooLarge = errors.New("D2 SVG output is too large")
)

type D2RenderRequest struct {
	Source string `json:"source"`
}

type D2RenderResponse struct {
	SVG    string `json:"svg"`
	Cached bool   `json:"cached"`
}

type D2RenderOptions struct {
	Command         string
	Timeout         time.Duration
	CacheTTL        time.Duration
	CacheMaxEntries int
	MaxSourceBytes  int
	MaxSVGBytes     int
	Now             func() time.Time
}

type d2Renderer struct {
	command         string
	timeout         time.Duration
	cacheTTL        time.Duration
	cacheMaxEntries int
	maxSourceBytes  int
	maxSVGBytes     int
	now             func() time.Time

	mu      sync.Mutex
	entries map[string]*list.Element
	lru     *list.List
	group   singleflight.Group
}

type d2CacheEntry struct {
	key       string
	svg       string
	expiresAt time.Time
}

type d2RenderGroupResult struct {
	svg    string
	cached bool
}

type d2RenderFailedError struct {
	message string
}

func (e d2RenderFailedError) Error() string {
	if e.message == "" {
		return ErrD2RenderFailed.Error()
	}
	return ErrD2RenderFailed.Error() + ": " + e.message
}

func (e d2RenderFailedError) Unwrap() error {
	return ErrD2RenderFailed
}

func newD2Renderer(opts D2RenderOptions) *d2Renderer {
	command := strings.TrimSpace(opts.Command)
	if command == "" {
		command = defaultD2Command
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultD2Timeout
	}
	cacheTTL := opts.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultD2CacheTTL
	}
	cacheMaxEntries := opts.CacheMaxEntries
	if cacheMaxEntries <= 0 {
		cacheMaxEntries = defaultD2CacheMaxEntries
	}
	maxSourceBytes := opts.MaxSourceBytes
	if maxSourceBytes <= 0 {
		maxSourceBytes = defaultD2MaxSourceBytes
	}
	maxSVGBytes := opts.MaxSVGBytes
	if maxSVGBytes <= 0 {
		maxSVGBytes = defaultD2MaxSVGBytes
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	return &d2Renderer{
		command:         command,
		timeout:         timeout,
		cacheTTL:        cacheTTL,
		cacheMaxEntries: cacheMaxEntries,
		maxSourceBytes:  maxSourceBytes,
		maxSVGBytes:     maxSVGBytes,
		now:             now,
		entries:         make(map[string]*list.Element),
		lru:             list.New(),
	}
}

func (a *App) RenderD2Diagram(ctx context.Context, req D2RenderRequest) (D2RenderResponse, error) {
	return a.d2Renderer.Render(ctx, req.Source)
}

func (r *d2Renderer) Render(ctx context.Context, source string) (D2RenderResponse, error) {
	if strings.TrimSpace(source) == "" {
		return D2RenderResponse{}, invalidInput("D2 source is required")
	}
	if len(source) > r.maxSourceBytes {
		return D2RenderResponse{}, invalidInput(fmt.Sprintf("D2 source exceeds %d bytes", r.maxSourceBytes))
	}

	key := d2SourceCacheKey(source)
	if svg, ok := r.cached(key, r.now()); ok {
		return D2RenderResponse{SVG: svg, Cached: true}, nil
	}

	value, err, _ := r.group.Do(key, func() (any, error) {
		if svg, ok := r.cached(key, r.now()); ok {
			return d2RenderGroupResult{svg: svg, cached: true}, nil
		}
		svg, err := r.render(ctx, source)
		if err != nil {
			return nil, err
		}
		r.store(key, svg, r.now())
		return d2RenderGroupResult{svg: svg}, nil
	})
	if err != nil {
		return D2RenderResponse{}, err
	}
	result, ok := value.(d2RenderGroupResult)
	if !ok {
		return D2RenderResponse{}, errors.New("D2 render returned unexpected result")
	}
	return D2RenderResponse{SVG: result.svg, Cached: result.cached}, nil
}

func D2RenderErrorMessage(err error) string {
	var renderErr d2RenderFailedError
	if errors.As(err, &renderErr) && renderErr.message != "" {
		return renderErr.message
	}
	switch {
	case errors.Is(err, ErrD2RenderTimeout):
		return ErrD2RenderTimeout.Error()
	case errors.Is(err, ErrD2CommandUnavailable):
		return ErrD2CommandUnavailable.Error()
	case errors.Is(err, ErrD2RenderFailed):
		return ErrD2RenderFailed.Error()
	default:
		return "D2 render failed"
	}
}

func (r *d2Renderer) render(ctx context.Context, source string) (string, error) {
	renderCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	timeoutSeconds := int(r.timeout.Round(time.Second).Seconds())
	if timeoutSeconds < 1 {
		timeoutSeconds = 1
	}
	cmd := exec.CommandContext(
		renderCtx,
		r.command,
		"--no-xml-tag",
		"--omit-version",
		"--timeout",
		strconv.Itoa(timeoutSeconds),
		"--stdout-format",
		"svg",
		"-",
		"-",
	)
	cmd.Stdin = strings.NewReader(source)
	var stdout limitedBuffer
	stdout.max = r.maxSVGBytes
	var stderr cappedBuffer
	stderr.max = maxD2StderrBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if stdout.exceeded || errors.Is(err, errD2OutputTooLarge) {
		return "", invalidInput(fmt.Sprintf("D2 SVG output exceeds %d bytes", r.maxSVGBytes))
	}
	if errors.Is(renderCtx.Err(), context.DeadlineExceeded) {
		return "", ErrD2RenderTimeout
	}
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return "", ErrD2CommandUnavailable
		}
		return "", d2RenderFailedError{message: d2RenderFailureMessage(err, stderr.String())}
	}

	svg := stdout.String()
	if !strings.HasPrefix(strings.TrimSpace(svg), "<svg") {
		return "", d2RenderFailedError{message: "D2 did not return SVG output"}
	}
	return svg, nil
}

func (r *d2Renderer) cached(key string, now time.Time) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	element, ok := r.entries[key]
	if !ok {
		return "", false
	}
	entry := element.Value.(*d2CacheEntry)
	if !now.Before(entry.expiresAt) {
		r.removeElementLocked(element)
		return "", false
	}
	r.lru.MoveToFront(element)
	return entry.svg, true
}

func (r *d2Renderer) store(key string, svg string, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cacheMaxEntries <= 0 {
		return
	}
	r.pruneExpiredLocked(now)
	if element, ok := r.entries[key]; ok {
		entry := element.Value.(*d2CacheEntry)
		entry.svg = svg
		entry.expiresAt = now.Add(r.cacheTTL)
		r.lru.MoveToFront(element)
		return
	}
	element := r.lru.PushFront(&d2CacheEntry{
		key:       key,
		svg:       svg,
		expiresAt: now.Add(r.cacheTTL),
	})
	r.entries[key] = element
	for len(r.entries) > r.cacheMaxEntries {
		r.removeElementLocked(r.lru.Back())
	}
}

func (r *d2Renderer) pruneExpiredLocked(now time.Time) {
	for element := r.lru.Back(); element != nil; {
		prev := element.Prev()
		entry := element.Value.(*d2CacheEntry)
		if now.Before(entry.expiresAt) {
			return
		}
		r.removeElementLocked(element)
		element = prev
	}
}

func (r *d2Renderer) removeElementLocked(element *list.Element) {
	if element == nil {
		return
	}
	entry := element.Value.(*d2CacheEntry)
	delete(r.entries, entry.key)
	r.lru.Remove(element)
}

func d2SourceCacheKey(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

func d2RenderFailureMessage(err error, stderr string) string {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		detail = err.Error()
	}
	return detail
}

type limitedBuffer struct {
	buf      bytes.Buffer
	max      int
	exceeded bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 {
		return b.buf.Write(p)
	}
	remaining := b.max - b.buf.Len()
	if remaining <= 0 {
		b.exceeded = true
		return 0, errD2OutputTooLarge
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.exceeded = true
		return remaining, errD2OutputTooLarge
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}

type cappedBuffer struct {
	buf bytes.Buffer
	max int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 || b.buf.Len() >= b.max {
		return len(p), nil
	}
	remaining := b.max - b.buf.Len()
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	return b.buf.String()
}
