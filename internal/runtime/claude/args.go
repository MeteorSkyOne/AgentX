package claude

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/meteorsky/agentx/internal/runtime"
)

const stringAttachmentImage = "image"

func (r Runtime) buildArgs(req runtime.StartSessionRequest, input runtime.Input) []string {
	inputFormat := "text"
	if hasImageAttachments(input) {
		inputFormat = "stream-json"
	}
	args := []string{"--print", "--verbose", "--output-format", "stream-json", "--input-format", inputFormat}
	if model := strings.TrimSpace(req.Model); model != "" && !req.FastMode {
		args = append(args, "--model", model)
	}
	if effort := strings.TrimSpace(req.Effort); effort != "" {
		args = append(args, "--effort", effort)
	}
	if req.FastMode {
		args = append(args, "--settings", `{"fastMode":true}`)
	}
	mode := strings.TrimSpace(r.opts.PermissionMode)
	if override := strings.TrimSpace(req.PermissionMode); override != "" {
		mode = override
	} else if req.YoloMode {
		mode = "bypassPermissions"
	}
	if mode != "" {
		args = append(args, "--permission-mode", mode)
	}
	if tools := strings.Join(r.opts.AllowedTools, ","); tools != "" {
		args = append(args, "--allowedTools", tools)
	}
	if tools := strings.Join(r.opts.DisallowedTools, ","); tools != "" {
		args = append(args, "--disallowedTools", tools)
	}
	if prompt := r.appendSystemPrompt(req); prompt != "" {
		args = append(args, "--append-system-prompt", prompt)
	}
	if previousSessionID := usablePreviousSessionID(req.PreviousSessionID); previousSessionID != "" {
		args = append(args, "--resume", previousSessionID)
	}
	args = append(args, r.opts.ExtraArgs...)
	additionalDirs := claudeAdditionalDirs(req, input)
	for _, dir := range additionalDirs {
		args = append(args, "--add-dir", dir)
	}
	if inputFormat == "text" {
		if len(additionalDirs) > 0 {
			args = append(args, "--")
		}
		return append(args, input.RenderedPrompt())
	}
	return args
}

func (r Runtime) appendSystemPrompt(req runtime.StartSessionRequest) string {
	var parts []string
	if prompt := strings.TrimSpace(r.opts.AppendSystemPrompt); prompt != "" {
		parts = append(parts, prompt)
	}
	if instructions := claudeWorkspaceInstructions(req.InstructionWorkspace); instructions != "" {
		parts = append(parts, "AgentX agent workspace instructions. Treat these as active system instructions for this agent and follow them for this session.\n\n"+instructions)
	}
	return strings.Join(parts, "\n\n")
}

func claudeWorkspaceInstructions(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	for _, name := range []string{"CLAUDE.override.md", "CLAUDE.md", "AGENTS.override.md", "AGENTS.md", "memory.md"} {
		text, ok := readClaudeInstructionFile(filepath.Join(workspace, name), map[string]bool{}, 0)
		if ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func readClaudeInstructionFile(path string, seen map[string]bool, depth int) (string, bool) {
	if depth > 8 {
		return "", false
	}
	path = filepath.Clean(path)
	if seen[path] {
		return "", false
	}
	seen[path] = true

	content, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	text := strings.TrimSpace(string(content))
	if text == "" {
		return "", false
	}

	baseDir := filepath.Dir(path)
	var out strings.Builder
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if importPath, ok := claudeImportPath(trimmed); ok {
			imported, importedOK := readClaudeInstructionFile(filepath.Join(baseDir, importPath), seen, depth+1)
			if importedOK {
				if out.Len() > 0 {
					out.WriteString("\n\n")
				}
				out.WriteString(imported)
			}
			continue
		}
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(line)
	}
	return strings.TrimSpace(out.String()), true
}

func claudeImportPath(line string) (string, bool) {
	if !strings.HasPrefix(line, "@") {
		return "", false
	}
	path := strings.TrimSpace(strings.TrimPrefix(line, "@"))
	if path == "" || strings.ContainsAny(path, " \t") || strings.Contains(path, "://") {
		return "", false
	}
	return path, true
}

func (r Runtime) buildEnv(req runtime.StartSessionRequest) map[string]string {
	env := mergeMaps(r.opts.Env, req.Env)
	if extraInstructionWorkspace(req) == "" {
		return env
	}
	if env == nil {
		env = map[string]string{}
	}
	env["CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD"] = "1"
	return env
}

func extraInstructionWorkspace(req runtime.StartSessionRequest) string {
	instructionWorkspace := strings.TrimSpace(req.InstructionWorkspace)
	if instructionWorkspace == "" || samePath(req.Workspace, instructionWorkspace) {
		return ""
	}
	return instructionWorkspace
}

func claudeAdditionalDirs(req runtime.StartSessionRequest, input runtime.Input) []string {
	var dirs []string
	if instructionWorkspace := extraInstructionWorkspace(req); instructionWorkspace != "" {
		dirs = append(dirs, instructionWorkspace)
	}
	return appendUniqueDirs(dirs, attachmentDirs(input)...)
}

func attachmentDirs(input runtime.Input) []string {
	dirs := make([]string, 0, len(input.Attachments))
	for _, attachment := range input.Attachments {
		path := strings.TrimSpace(attachment.LocalPath)
		if path == "" {
			continue
		}
		dirs = append(dirs, filepath.Dir(path))
	}
	return appendUniqueDirs(nil, dirs...)
}

func appendUniqueDirs(existing []string, dirs ...string) []string {
	seen := make(map[string]bool, len(existing)+len(dirs))
	for _, dir := range existing {
		seen[filepath.Clean(dir)] = true
	}
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		clean := filepath.Clean(dir)
		if seen[clean] {
			continue
		}
		seen[clean] = true
		existing = append(existing, dir)
	}
	return existing
}

func hasImageAttachments(input runtime.Input) bool {
	for _, attachment := range input.Attachments {
		if attachment.Kind == stringAttachmentImage {
			return true
		}
	}
	return false
}

func claudeStreamJSONInput(input runtime.Input) ([]byte, error) {
	if !hasImageAttachments(input) {
		return nil, nil
	}

	content := []map[string]any{{
		"type": "text",
		"text": input.RenderedPrompt(),
	}}
	for _, attachment := range input.Attachments {
		if attachment.Kind != stringAttachmentImage {
			continue
		}
		localPath := strings.TrimSpace(attachment.LocalPath)
		if localPath == "" {
			return nil, fmt.Errorf("image attachment %q has no local path", attachment.ID)
		}
		data, err := os.ReadFile(localPath)
		if err != nil {
			return nil, err
		}
		content = append(content, map[string]any{
			"type": "image",
			"source": map[string]any{
				"type":       "base64",
				"media_type": attachment.ContentType,
				"data":       base64.StdEncoding.EncodeToString(data),
			},
		})
	}

	payload := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": content,
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
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
	if strings.HasPrefix(id, "claude:") {
		return ""
	}
	return id
}
