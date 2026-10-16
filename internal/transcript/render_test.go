package transcript

import (
	"strings"
	"testing"

	"github.com/meteorsky/agentx/internal/domain"
)

func TestCleanBodyDropsProcessBreakMarkers(t *testing.T) {
	body := "First step.\n\n<!-- process-break:2 -->\n\n\n\n<!-- process-break:2 -->\n\nSecond step."
	if got := cleanBody(body); got != "First step.\n\nSecond step." {
		t.Fatalf("cleanBody = %q", got)
	}
}

func TestRenderStopsAtByteBudgetAndCutsOversizedFirstMessage(t *testing.T) {
	post := Post{ID: "thr_1", Title: "Budget", Messages: []Message{
		{Index: 1, ID: "msg_1", SenderType: domain.SenderUser, SenderName: "Alice", Body: strings.Repeat("a", 100)},
		{Index: 2, ID: "msg_2", SenderType: domain.SenderBot, SenderName: "Bot", Body: strings.Repeat("b", 100)},
		{Index: 3, ID: "msg_3", SenderType: domain.SenderUser, SenderName: "Alice", Body: strings.Repeat("界", 400)},
	}}

	page := Render(post, RenderOptions{MaxBytes: 300})
	if page.First != 1 || page.Last != 2 || page.Truncated {
		t.Fatalf("page = %d-%d truncated=%v, want 1-2", page.First, page.Last, page.Truncated)
	}
	if !strings.Contains(page.Text, "- Showing: messages 1-2") {
		t.Fatalf("missing showing line:\n%s", page.Text)
	}

	page = Render(post, RenderOptions{Offset: 2, MaxBytes: 300})
	if page.First != 3 || page.Last != 3 || !page.Truncated {
		t.Fatalf("page = %d-%d truncated=%v, want truncated 3", page.First, page.Last, page.Truncated)
	}
	if !strings.Contains(page.Text, "[message truncated]") || strings.ContainsRune(page.Text, '�') {
		t.Fatalf("oversized message not cut cleanly:\n%s", page.Text)
	}

	// Following Resume across pages must deliver the whole oversized message.
	delivered := 0
	resume := 0
	for i := 0; i < 20; i++ {
		page = Render(post, RenderOptions{Offset: 2, MaxBytes: 300, Resume: resume})
		delivered += strings.Count(page.Text, "界")
		if !page.Truncated {
			break
		}
		if page.Resume <= resume {
			t.Fatalf("resume did not advance: %d -> %d", resume, page.Resume)
		}
		resume = page.Resume
	}
	if page.Truncated || delivered != 400 {
		t.Fatalf("delivered %d of 400 characters, truncated=%v", delivered, page.Truncated)
	}

	if full := Markdown(post, RenderOptions{}); strings.Contains(full, "Showing:") || !strings.Contains(full, "## [3] Alice (user)") {
		t.Fatalf("unbounded markdown:\n%s", full)
	}
}

func TestSearchMatchesAllTermsCaseInsensitively(t *testing.T) {
	post := Post{Messages: []Message{
		{Index: 1, Body: "Deploy the Rollback script"},
		{Index: 2, Body: "rollback only"},
		{Index: 3, Body: "nothing here", Attachments: []Attachment{{Filename: "deploy-rollback.md"}}},
	}}
	matches := Search(post, "ROLLBACK deploy", SearchOptions{})
	if len(matches) != 2 || matches[0].Message.Index != 1 || matches[1].Message.Index != 3 {
		t.Fatalf("matches = %#v", matches)
	}
}

func TestRenderShowsThinkingOnceAndFallsBackWithoutProcess(t *testing.T) {
	post := Post{ID: "thr_1", Messages: []Message{
		{Index: 1, ID: "msg_1", SenderType: domain.SenderBot, SenderName: "Bot", Body: "Done.",
			Thinking: "Plan the deploy carefully.",
			Process: []domain.ProcessItem{
				{Type: "thinking", Text: "Plan the deploy carefully."},
				{Type: "tool_call", ToolName: "Bash", Input: map[string]any{"command": "make deploy"}},
			}},
		{Index: 2, ID: "msg_2", SenderType: domain.SenderBot, SenderName: "Bot", Body: "Ok.", Thinking: "Legacy reasoning only."},
	}}
	text := Markdown(post, RenderOptions{IncludeProcess: true})
	if got := strings.Count(text, "Plan the deploy carefully."); got != 1 {
		t.Fatalf("thinking rendered %d times, want 1:\n%s", got, text)
	}
	for _, want := range []string{"tool `Bash`", "Legacy reasoning only."} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q:\n%s", want, text)
		}
	}
	if plain := Markdown(post, RenderOptions{}); strings.Contains(plain, "Plan the deploy") || strings.Contains(plain, "Process") {
		t.Fatalf("process leaked without IncludeProcess:\n%s", plain)
	}
}

func TestSearchIncludesProcessOnlyWhenAsked(t *testing.T) {
	post := Post{Messages: []Message{
		{Index: 1, Body: "Shipped.", Process: []domain.ProcessItem{
			{Type: "tool_call", ToolName: "Bash", Input: map[string]any{"command": "cat <<EOF | kubectl rollout restart"}},
		}},
	}}
	if matches := Search(post, "kubectl", SearchOptions{}); len(matches) != 0 {
		t.Fatalf("body-only search matched process: %#v", matches)
	}
	matches := Search(post, "kubectl", SearchOptions{IncludeProcess: true})
	if len(matches) != 1 || !matches[0].InProcess || !strings.Contains(matches[0].Snippet, "<<EOF | kubectl rollout restart") {
		t.Fatalf("process search = %#v", matches)
	}
}
