package codex

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/meteorsky/agentx/internal/runtime"
)

func (r Runtime) buildArgs(req runtime.StartSessionRequest, input runtime.Input) []string {
	args, _ := r.buildArgsAndStdin(req, input)
	return args
}

func (r Runtime) buildArgsAndStdin(req runtime.StartSessionRequest, input runtime.Input) ([]string, []byte) {
	var args []string
	args = append(args, "exec")
	previousSessionID := usablePreviousSessionID(req.PreviousSessionID)
	additionalDirs := attachmentDirs(input)
	if previousSessionID != "" {
		for _, dir := range additionalDirs {
			args = append(args, "--add-dir", dir)
		}
	}
	if previousSessionID != "" {
		args = append(args, "resume")
	}

	args = append(args, "--json")
	if model := strings.TrimSpace(req.Model); model != "" {
		args = append(args, "--model", model)
	}
	if effort := strings.TrimSpace(req.Effort); effort != "" {
		args = append(args, "-c", `model_reasoning_effort="`+effort+`"`)
	}
	if req.FastMode {
		args = append(args, "-c", `service_tier="fast"`, "-c", "features.fast_mode=true")
	}
	if instructions := codexDeveloperInstructions(req); instructions != "" {
		args = append(args, "-c", "developer_instructions="+strconv.Quote(instructions))
	}
	if req.YoloMode {
		args = append(args, "--yolo")
	} else if r.opts.BypassSandbox {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	} else if r.opts.FullAuto {
		args = append(args, "--full-auto")
	}
	if r.opts.SkipGitRepoCheck {
		args = append(args, "--skip-git-repo-check")
	}
	if previousSessionID == "" {
		for _, dir := range additionalDirs {
			args = append(args, "--add-dir", dir)
		}
	}
	for _, attachment := range input.Attachments {
		if isImageAttachment(attachment) {
			if path := strings.TrimSpace(attachment.LocalPath); path != "" {
				args = append(args, "--image", path)
			}
		}
	}
	args = append(args, r.opts.ExtraArgs...)
	prompt := input.RenderedPrompt()
	if hasImageAttachments(input) {
		if previousSessionID != "" {
			args = append(args, "--", previousSessionID, "-")
		} else {
			args = append(args, "--", "-")
		}
		return args, []byte(prompt)
	}
	if previousSessionID != "" {
		args = append(args, previousSessionID)
	}
	return append(args, prompt), nil
}

func hasImageAttachments(input runtime.Input) bool {
	for _, attachment := range input.Attachments {
		if isImageAttachment(attachment) {
			return true
		}
	}
	return false
}

func isImageAttachment(attachment runtime.Attachment) bool {
	return attachment.Kind == "image"
}

func attachmentDirs(input runtime.Input) []string {
	dirs := make([]string, 0, len(input.Attachments))
	seen := make(map[string]bool, len(input.Attachments))
	for _, attachment := range input.Attachments {
		path := strings.TrimSpace(attachment.LocalPath)
		if path == "" {
			continue
		}
		dir := filepath.Clean(filepath.Dir(path))
		if seen[dir] {
			continue
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	return dirs
}

func codexDeveloperInstructions(req runtime.StartSessionRequest) string {
	instructionWorkspace := strings.TrimSpace(req.InstructionWorkspace)
	if instructionWorkspace == "" || samePath(req.Workspace, instructionWorkspace) {
		return ""
	}
	for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
		content, err := os.ReadFile(filepath.Join(instructionWorkspace, name))
		if err != nil {
			continue
		}
		if text := strings.TrimSpace(string(content)); text != "" {
			return text
		}
	}
	return ""
}

func samePath(left string, right string) bool {
	left = strings.TrimSpace(left)
	if left == "" {
		left = "."
	}
	right = strings.TrimSpace(right)
	if right == "" {
		right = "."
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func usablePreviousSessionID(id string) string {
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, "codex:") {
		return ""
	}
	return id
}
