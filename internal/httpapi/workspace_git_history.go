package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

func parseWorkspaceGitHistoryOptions(r *http.Request) (workspaceGitHistoryOptions, error) {
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = workspaceGitHistoryRepository
	}
	if mode != workspaceGitHistoryRepository && mode != workspaceGitHistoryFile {
		return workspaceGitHistoryOptions{}, errors.New("mode must be repository or file")
	}
	limit := workspaceGitHistoryDefaultLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return workspaceGitHistoryOptions{}, errors.New("limit must be a positive integer")
		}
		limit = parsed
	}
	if limit > workspaceGitHistoryMaxLimit {
		limit = workspaceGitHistoryMaxLimit
	}
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			return workspaceGitHistoryOptions{}, errors.New("offset must be zero or a positive integer")
		}
		offset = parsed
	}
	path := ""
	if mode == workspaceGitHistoryFile {
		cleaned, err := cleanWorkspaceRelPath(r.URL.Query().Get("path"))
		if err != nil {
			return workspaceGitHistoryOptions{}, errors.New("invalid path")
		}
		if cleaned == "" {
			return workspaceGitHistoryOptions{}, errors.New("path is required for file history")
		}
		path = cleaned
	}
	return workspaceGitHistoryOptions{
		mode:   mode,
		path:   path,
		query:  strings.TrimSpace(r.URL.Query().Get("q")),
		limit:  limit,
		offset: offset,
	}, nil
}

func workspaceGitHistory(ctx context.Context, gitCtx workspaceGitContext, opts workspaceGitHistoryOptions) ([]workspaceGitCommit, bool, error) {
	args := []string{
		"log",
		"--format=%H%x1f%h%x1f%an%x1f%ae%x1f%aI%x1f%s",
	}
	if opts.query == "" {
		args = append(args,
			"--max-count", strconv.Itoa(opts.limit+1),
			"--skip", strconv.Itoa(opts.offset),
		)
	}
	args = append(args, "HEAD", "--")
	switch opts.mode {
	case workspaceGitHistoryFile:
		args = append(args, workspaceGitRepoPath(gitCtx, opts.path))
	default:
		if gitCtx.prefix != "" {
			args = append(args, gitCtx.prefix)
		}
	}
	output, err := workspaceGitOutput(ctx, gitCtx.repoRoot, args...)
	if err != nil {
		return nil, false, err
	}
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	commits := make([]workspaceGitCommit, 0, min(len(lines), opts.limit))
	query := strings.ToLower(opts.query)
	matched := 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\x1f", 6)
		if len(fields) != 6 {
			continue
		}
		commit := workspaceGitCommit{
			SHA:         fields[0],
			ShortSHA:    fields[1],
			AuthorName:  fields[2],
			AuthorEmail: fields[3],
			AuthoredAt:  fields[4],
			Subject:     fields[5],
		}
		if query != "" {
			if !workspaceGitCommitMatchesQuery(commit, query) {
				continue
			}
			if matched < opts.offset {
				matched++
				continue
			}
			matched++
		}
		commits = append(commits, commit)
		if query != "" && len(commits) > opts.limit {
			break
		}
	}
	hasMore := len(commits) > opts.limit
	if hasMore {
		commits = commits[:opts.limit]
	}
	if commits == nil {
		commits = []workspaceGitCommit{}
	}
	return commits, hasMore, nil
}

func workspaceGitCommitMatchesQuery(commit workspaceGitCommit, query string) bool {
	if strings.Contains(strings.ToLower(commit.SHA), query) {
		return true
	}
	if strings.Contains(strings.ToLower(commit.ShortSHA), query) {
		return true
	}
	if strings.Contains(strings.ToLower(commit.Subject), query) {
		return true
	}
	if strings.Contains(strings.ToLower(commit.AuthorName), query) {
		return true
	}
	return strings.Contains(strings.ToLower(commit.AuthorEmail), query)
}
