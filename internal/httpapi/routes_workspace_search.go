package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

const (
	workspaceSearchModeFiles       = "files"
	workspaceSearchModeContent     = "content"
	workspaceSearchDefaultLimit    = 200
	workspaceSearchMaxLimit        = 500
	workspaceSearchCommandTimeout  = 5 * time.Second
	workspaceSearchMaxFileBytes    = 2 * 1024 * 1024
	workspaceSearchMaxPreviewRunes = 320
)

var workspaceSearchRgPath = "rg"

type workspaceSearchResponse struct {
	Query     string                  `json:"query"`
	Mode      string                  `json:"mode"`
	Engine    string                  `json:"engine"`
	Truncated bool                    `json:"truncated"`
	Results   []workspaceSearchResult `json:"results"`
}

type workspaceSearchResult struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	LineNumber int    `json:"line_number,omitempty"`
	Column     int    `json:"column,omitempty"`
	Preview    string `json:"preview,omitempty"`
}

type workspaceSearchOptions struct {
	query         string
	mode          string
	caseSensitive bool
	regex         bool
	wholeWord     bool
	limit         int
}

func (s *Server) handleWorkspaceSearch(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	workspace, ok, err := s.authorizedWorkspace(r, userID, chi.URLParam(r, "workspaceID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	root, err := safeWorkspacePath(workspace.Path, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	opts, err := parseWorkspaceSearchOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	results, engine, truncated, err := searchWorkspace(r.Context(), root, opts)
	if err != nil {
		if errors.Is(err, errWorkspaceSearchInvalidPattern) {
			writeError(w, http.StatusBadRequest, "invalid search pattern")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, workspaceSearchResponse{
		Query:     opts.query,
		Mode:      opts.mode,
		Engine:    engine,
		Truncated: truncated,
		Results:   results,
	})
}

func parseWorkspaceSearchOptions(r *http.Request) (workspaceSearchOptions, error) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		return workspaceSearchOptions{}, errors.New("query is required")
	}
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = workspaceSearchModeFiles
	}
	if mode != workspaceSearchModeFiles && mode != workspaceSearchModeContent {
		return workspaceSearchOptions{}, errors.New("mode must be files or content")
	}
	limit := workspaceSearchDefaultLimit
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 {
			return workspaceSearchOptions{}, errors.New("limit must be a positive integer")
		}
		limit = parsed
	}
	if limit > workspaceSearchMaxLimit {
		limit = workspaceSearchMaxLimit
	}
	return workspaceSearchOptions{
		query:         query,
		mode:          mode,
		caseSensitive: r.URL.Query().Get("case_sensitive") == "1" || r.URL.Query().Get("case_sensitive") == "true",
		regex:         r.URL.Query().Get("regex") == "1" || r.URL.Query().Get("regex") == "true",
		wholeWord:     r.URL.Query().Get("whole_word") == "1" || r.URL.Query().Get("whole_word") == "true",
		limit:         limit,
	}, nil
}

var errWorkspaceSearchInvalidPattern = errors.New("invalid search pattern")

func searchWorkspace(ctx context.Context, root string, opts workspaceSearchOptions) ([]workspaceSearchResult, string, bool, error) {
	if _, err := exec.LookPath(workspaceSearchRgPath); err == nil {
		results, truncated, err := searchWorkspaceWithRg(ctx, root, opts)
		if err == nil {
			return results, "rg", truncated, nil
		}
		if errors.Is(err, errWorkspaceSearchInvalidPattern) {
			return nil, "", false, err
		}
	} else if opts.regex {
		if _, err := compileWorkspaceSearchPattern(opts); err != nil {
			return nil, "", false, err
		}
	}
	results, truncated, err := searchWorkspaceFallback(ctx, root, opts)
	return results, "fallback", truncated, err
}

func searchWorkspaceWithRg(ctx context.Context, root string, opts workspaceSearchOptions) ([]workspaceSearchResult, bool, error) {
	if opts.mode == workspaceSearchModeContent {
		return searchWorkspaceContentWithRg(ctx, root, opts)
	}
	return searchWorkspaceFilesWithRg(ctx, root, opts)
}

func searchWorkspaceFilesWithRg(ctx context.Context, root string, opts workspaceSearchOptions) ([]workspaceSearchResult, bool, error) {
	matcher, err := compileWorkspaceSearchPattern(opts)
	if err != nil {
		return nil, false, err
	}
	searchCtx, cancel := context.WithTimeout(ctx, workspaceSearchCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(searchCtx, workspaceSearchRgPath, "--files", "--color", "never", "--no-messages")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if searchCtx.Err() != nil {
			return nil, false, searchCtx.Err()
		}
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("rg --files failed: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	var results []workspaceSearchResult
	lines := strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n")
	for _, line := range lines {
		path := normalizeWorkspaceSearchPath(line)
		if path == "" {
			continue
		}
		if !matcher(path) && !matcher(filepath.Base(path)) {
			continue
		}
		results = append(results, workspaceSearchResult{Path: path, Name: filepath.Base(path)})
		if len(results) >= opts.limit {
			return results, true, nil
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Path < results[j].Path
	})
	return results, false, nil
}

func searchWorkspaceContentWithRg(ctx context.Context, root string, opts workspaceSearchOptions) ([]workspaceSearchResult, bool, error) {
	args := []string{"--json", "--line-number", "--column", "--color", "never", "--no-messages"}
	if !opts.caseSensitive {
		args = append(args, "--ignore-case")
	}
	if !opts.regex {
		args = append(args, "--fixed-strings")
	}
	if opts.wholeWord {
		args = append(args, "--word-regexp")
	}
	args = append(args, "--", opts.query, ".")

	searchCtx, cancel := context.WithTimeout(ctx, workspaceSearchCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(searchCtx, workspaceSearchRgPath, args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	if err := cmd.Start(); err != nil {
		return nil, false, err
	}
	results, truncated, scanErr := scanRipgrepJSON(stdout, opts.limit)
	if truncated && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if scanErr != nil {
		return nil, false, scanErr
	}
	if waitErr != nil && !truncated {
		if searchCtx.Err() != nil {
			return nil, false, searchCtx.Err()
		}
		if exitErr, ok := waitErr.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return results, false, nil
		}
		if opts.regex && strings.TrimSpace(stderr.String()) != "" {
			return nil, false, errWorkspaceSearchInvalidPattern
		}
		return nil, false, fmt.Errorf("rg search failed: %s: %w", strings.TrimSpace(stderr.String()), waitErr)
	}
	return results, truncated, nil
}

type ripgrepJSONLine struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
		Submatches []struct {
			Start int `json:"start"`
		} `json:"submatches"`
	} `json:"data"`
}

func scanRipgrepJSON(reader io.Reader, limit int) ([]workspaceSearchResult, bool, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	results := make([]workspaceSearchResult, 0, min(limit, 64))
	for scanner.Scan() {
		var line ripgrepJSONLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		if line.Type != "match" {
			continue
		}
		path := normalizeWorkspaceSearchPath(line.Data.Path.Text)
		if path == "" {
			continue
		}
		column := 1
		if len(line.Data.Submatches) > 0 && line.Data.Submatches[0].Start >= 0 {
			column = utf8.RuneCountInString(line.Data.Lines.Text[:min(line.Data.Submatches[0].Start, len(line.Data.Lines.Text))]) + 1
		}
		results = append(results, workspaceSearchResult{
			Path:       path,
			Name:       filepath.Base(path),
			LineNumber: line.Data.LineNumber,
			Column:     column,
			Preview:    workspaceSearchPreview(line.Data.Lines.Text),
		})
		if len(results) >= limit {
			return results, true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}
	return results, false, nil
}

func searchWorkspaceFallback(ctx context.Context, root string, opts workspaceSearchOptions) ([]workspaceSearchResult, bool, error) {
	matcher, err := compileWorkspaceSearchPattern(opts)
	if err != nil {
		return nil, false, err
	}
	lineMatcher, err := compileWorkspaceSearchLineMatcher(opts)
	if err != nil {
		return nil, false, err
	}
	searchCtx, cancel := context.WithTimeout(ctx, workspaceSearchCommandTimeout)
	defer cancel()
	var results []workspaceSearchResult
	truncated := false
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := searchCtx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			return nil
		}
		if opts.mode == workspaceSearchModeFiles {
			if matcher(rel) || matcher(entry.Name()) {
				results = append(results, workspaceSearchResult{Path: rel, Name: entry.Name()})
			}
		} else {
			matches, err := fallbackSearchFile(path, rel, entry.Name(), lineMatcher)
			if err == nil {
				results = append(results, matches...)
			}
		}
		if len(results) >= opts.limit {
			results = results[:opts.limit]
			truncated = true
			return errWorkspaceSearchLimitReached
		}
		return nil
	})
	if errors.Is(err, errWorkspaceSearchLimitReached) {
		return results, truncated, nil
	}
	if err != nil {
		return nil, false, err
	}
	return results, truncated, nil
}

var errWorkspaceSearchLimitReached = errors.New("workspace search limit reached")

func fallbackSearchFile(path string, relPath string, name string, lineMatcher func(string) int) ([]workspaceSearchResult, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > workspaceSearchMaxFileBytes {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.Contains(data, []byte{0}) || !utf8.Valid(data) {
		return nil, errors.New("binary file")
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	results := make([]workspaceSearchResult, 0)
	for index, line := range lines {
		column := lineMatcher(line)
		if column == 0 {
			continue
		}
		results = append(results, workspaceSearchResult{
			Path:       relPath,
			Name:       name,
			LineNumber: index + 1,
			Column:     column,
			Preview:    workspaceSearchPreview(line),
		})
	}
	return results, nil
}

func compileWorkspaceSearchPattern(opts workspaceSearchOptions) (func(string) bool, error) {
	query := opts.query
	if opts.regex {
		pattern := query
		if opts.wholeWord {
			pattern = `\b(?:` + pattern + `)\b`
		}
		if !opts.caseSensitive {
			pattern = `(?i)` + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, errWorkspaceSearchInvalidPattern
		}
		return re.MatchString, nil
	}
	needle := query
	if !opts.caseSensitive {
		needle = strings.ToLower(needle)
	}
	return func(value string) bool {
		haystack := value
		if !opts.caseSensitive {
			haystack = strings.ToLower(haystack)
		}
		if !opts.wholeWord {
			return strings.Contains(haystack, needle)
		}
		start := 0
		for {
			index := strings.Index(haystack[start:], needle)
			if index < 0 {
				return false
			}
			index += start
			end := index + len(needle)
			if isWorkspaceSearchWordBoundary(haystack, index) && isWorkspaceSearchWordBoundary(haystack, end) {
				return true
			}
			start = index + 1
		}
	}, nil
}

func compileWorkspaceSearchLineMatcher(opts workspaceSearchOptions) (func(string) int, error) {
	if opts.mode != workspaceSearchModeContent {
		return func(string) int { return 0 }, nil
	}
	if opts.regex {
		pattern := opts.query
		if opts.wholeWord {
			pattern = `\b(?:` + pattern + `)\b`
		}
		if !opts.caseSensitive {
			pattern = `(?i)` + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, errWorkspaceSearchInvalidPattern
		}
		return func(value string) int {
			match := re.FindStringIndex(value)
			if match == nil {
				return 0
			}
			return utf8.RuneCountInString(value[:match[0]]) + 1
		}, nil
	}
	needle := opts.query
	if !opts.caseSensitive {
		needle = strings.ToLower(needle)
	}
	return func(value string) int {
		haystack := value
		if !opts.caseSensitive {
			haystack = strings.ToLower(haystack)
		}
		start := 0
		for {
			index := strings.Index(haystack[start:], needle)
			if index < 0 {
				return 0
			}
			index += start
			end := index + len(needle)
			if !opts.wholeWord || (isWorkspaceSearchWordBoundary(haystack, index) && isWorkspaceSearchWordBoundary(haystack, end)) {
				return utf8.RuneCountInString(value[:index]) + 1
			}
			start = index + 1
		}
	}, nil
}

func isWorkspaceSearchWordBoundary(value string, byteIndex int) bool {
	if byteIndex <= 0 || byteIndex >= len(value) {
		return true
	}
	before, _ := utf8.DecodeLastRuneInString(value[:byteIndex])
	after, _ := utf8.DecodeRuneInString(value[byteIndex:])
	return !isWorkspaceSearchWordRune(before) || !isWorkspaceSearchWordRune(after)
}

func isWorkspaceSearchWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func normalizeWorkspaceSearchPath(path string) string {
	path = strings.TrimSpace(filepath.ToSlash(path))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}

func workspaceSearchPreview(line string) string {
	line = strings.TrimRight(strings.ReplaceAll(line, "\r", ""), "\n")
	runes := []rune(line)
	if len(runes) <= workspaceSearchMaxPreviewRunes {
		return line
	}
	return string(runes[:workspaceSearchMaxPreviewRunes])
}
