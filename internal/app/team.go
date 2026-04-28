package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
)

type teamBudget struct {
	MaxBatches int
	MaxRuns    int
}

type teamBatchItem struct {
	Target          ConversationAgentContext
	Prompt          string
	SourceMessageID string
}

type teamBatchResult struct {
	Index   int
	Target  ConversationAgentContext
	Message domain.Message
	Err     error
}

type teamTranscriptEntry struct {
	Handle string
	Name   string
	Body   string
}

type teamHandoff struct {
	Target          ConversationAgentContext
	Prompt          string
	SourceMessageID string
}

func (a *App) dispatchAgentRunsForMessage(ctx context.Context, message domain.Message, scope conversationScope, agents []ConversationAgentContext) {
	if len(agents) == 0 {
		return
	}
	if targets := mentionedAgentsForBody(agents, message.Body); len(targets) > 0 {
		go a.runAgentTeamForMessage(ctx, message, scope, agents, targets)
		return
	}
	for _, agent := range agents {
		go a.runAgentForMessage(ctx, message, agent)
	}
}

func (a *App) runAgentTeamForMessage(ctx context.Context, rootMessage domain.Message, scope conversationScope, roster []ConversationAgentContext, initialTargets []ConversationAgentContext) {
	budget := teamBudgetForScope(scope)
	sessionID := id.New("team")
	leader := initialTargets[0]
	runsUsed := 0
	batchesUsed := 0
	var transcript []teamTranscriptEntry
	discussionStarted := len(initialTargets) > 1

	initialItems := make([]teamBatchItem, 0, len(initialTargets))
	for _, target := range initialTargets {
		if runsUsed >= budget.MaxRuns {
			break
		}
		initialItems = append(initialItems, teamBatchItem{Target: target, Prompt: rootMessage.Body})
		runsUsed++
	}
	if len(initialItems) == 0 {
		return
	}

	batchesUsed++
	initialResults := a.runTeamBatch(ctx, rootMessage, roster, leader, sessionID, initialItems, batchesUsed, teamPhaseForInitialBatch(discussionStarted), discussionStarted, budget, runsUsed, batchesUsed)
	transcript = append(transcript, transcriptEntriesFromResults(initialResults)...)
	handoffs := handoffsFromResults(roster, initialResults)
	if len(handoffs) > 0 {
		discussionStarted = true
	}

	stopReason := "no new team handoffs"
	for len(handoffs) > 0 {
		if batchesUsed >= budget.MaxBatches {
			stopReason = "team batch budget reached"
			break
		}
		if runsUsed >= budget.MaxRuns {
			stopReason = "team run budget reached"
			break
		}

		items := make([]teamBatchItem, 0, len(handoffs))
		for _, handoff := range handoffs {
			if runsUsed >= budget.MaxRuns {
				stopReason = "team run budget reached"
				break
			}
			items = append(items, teamBatchItem{
				Target:          handoff.Target,
				Prompt:          handoff.Prompt,
				SourceMessageID: handoff.SourceMessageID,
			})
			runsUsed++
		}
		if len(items) == 0 {
			break
		}

		batchesUsed++
		results := a.runTeamBatch(ctx, rootMessage, roster, leader, sessionID, items, batchesUsed, "discussion", true, budget, runsUsed, batchesUsed)
		transcript = append(transcript, transcriptEntriesFromResults(results)...)
		handoffs = handoffsFromResults(roster, results)
		if len(handoffs) == 0 {
			stopReason = "no new team handoffs"
		}
	}

	if !discussionStarted {
		return
	}
	summaryPrompt := teamSummaryPrompt(rootMessage, transcript, stopReason)
	team := &domain.TeamMetadata{
		SessionID:     sessionID,
		RootMessageID: rootMessage.ID,
		LeaderAgentID: leader.Agent.ID,
		Phase:         "summary",
		Turn:          batchesUsed + 1,
	}
	result := make(chan agentRunResult, 1)
	a.runAgentForMessageWithTarget(ctx, rootMessage, leader, id.New("run"), agentRunOptions{
		Prompt: summaryPrompt,
		Context: teamProtocolContext(teamProtocolContextInput{
			RootMessage:      rootMessage,
			Roster:           roster,
			Leader:           leader,
			Speaker:          leader,
			Budget:           budget,
			RunsUsed:         runsUsed,
			BatchesUsed:      batchesUsed,
			SummaryMode:      true,
			DiscussionDigest: teamTranscriptText(transcript),
		}),
		Result: result,
		Team:   team,
	})
}

func teamBudgetForScope(scope conversationScope) teamBudget {
	maxBatches := scope.channel.TeamMaxBatches
	if maxBatches <= 0 {
		maxBatches = DefaultChannelTeamMaxBatches
	}
	maxRuns := scope.channel.TeamMaxRuns
	if maxRuns <= 0 {
		maxRuns = DefaultChannelTeamMaxRuns
	}
	return teamBudget{MaxBatches: maxBatches, MaxRuns: maxRuns}
}

func teamPhaseForInitialBatch(discussionStarted bool) string {
	if discussionStarted {
		return "discussion"
	}
	return "leader"
}

func (a *App) runTeamBatch(ctx context.Context, rootMessage domain.Message, roster []ConversationAgentContext, leader ConversationAgentContext, sessionID string, items []teamBatchItem, turn int, phase string, includeTeamMetadata bool, budget teamBudget, runsUsed int, batchesUsed int) []teamBatchResult {
	results := make([]teamBatchResult, len(items))
	var wg sync.WaitGroup
	out := make(chan teamBatchResult, len(items))
	for index, item := range items {
		index, item := index, item
		wg.Add(1)
		go func() {
			defer wg.Done()
			var team *domain.TeamMetadata
			if includeTeamMetadata {
				team = &domain.TeamMetadata{
					SessionID:       sessionID,
					RootMessageID:   rootMessage.ID,
					LeaderAgentID:   leader.Agent.ID,
					Phase:           phase,
					Turn:            turn,
					SourceMessageID: item.SourceMessageID,
				}
			}
			result := make(chan agentRunResult, 1)
			a.runAgentForMessageWithTarget(ctx, rootMessage, item.Target, id.New("run"), agentRunOptions{
				Prompt: item.Prompt,
				Context: teamProtocolContext(teamProtocolContextInput{
					RootMessage: rootMessage,
					Roster:      roster,
					Leader:      leader,
					Speaker:     item.Target,
					Budget:      budget,
					RunsUsed:    runsUsed,
					BatchesUsed: batchesUsed,
				}),
				Result: result,
				Team:   team,
			})
			runResult := <-result
			out <- teamBatchResult{Index: index, Target: item.Target, Message: runResult.Message, Err: runResult.Err}
		}()
	}
	wg.Wait()
	close(out)
	for result := range out {
		results[result.Index] = result
	}
	return results
}

func transcriptEntriesFromResults(results []teamBatchResult) []teamTranscriptEntry {
	entries := make([]teamTranscriptEntry, 0, len(results))
	for _, result := range results {
		if result.Err != nil || strings.TrimSpace(result.Message.Body) == "" {
			continue
		}
		entries = append(entries, teamTranscriptEntry{
			Handle: result.Target.Agent.Handle,
			Name:   result.Target.Agent.Name,
			Body:   result.Message.Body,
		})
	}
	return entries
}

func handoffsFromResults(roster []ConversationAgentContext, results []teamBatchResult) []teamHandoff {
	var handoffs []teamHandoff
	for _, result := range results {
		if result.Err != nil {
			continue
		}
		handoffs = append(handoffs, teamHandoffsFromBody(roster, result.Target.Agent.ID, result.Message.ID, result.Message.Body)...)
	}
	return dedupeTeamHandoffs(handoffs)
}

func dedupeTeamHandoffs(handoffs []teamHandoff) []teamHandoff {
	result := make([]teamHandoff, 0, len(handoffs))
	seen := make(map[string]int, len(handoffs))
	for _, handoff := range handoffs {
		key := handoff.Target.Agent.ID
		if index, ok := seen[key]; ok {
			if prompt := strings.TrimSpace(handoff.Prompt); prompt != "" {
				result[index].Prompt = strings.TrimSpace(result[index].Prompt + "\n" + prompt)
			}
			continue
		}
		seen[key] = len(result)
		result = append(result, handoff)
	}
	return result
}

func mentionedAgentsForBody(agents []ConversationAgentContext, body string) []ConversationAgentContext {
	known := agentsByHandle(agents)
	targets := make([]ConversationAgentContext, 0)
	seen := make(map[string]struct{})
	for _, mention := range agentMentions(body) {
		target, ok := known[strings.ToLower(mention)]
		if !ok {
			continue
		}
		if _, ok := seen[target.Agent.ID]; ok {
			continue
		}
		seen[target.Agent.ID] = struct{}{}
		targets = append(targets, target)
	}
	return targets
}

func teamHandoffsFromBody(agents []ConversationAgentContext, speakerAgentID string, sourceMessageID string, body string) []teamHandoff {
	known := agentsByHandle(agents)
	var handoffs []teamHandoff
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || strings.HasPrefix(trimmed, ">") {
			continue
		}
		line = strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(line, "@") {
			continue
		}
		handle, task, ok := parseTeamHandoffLine(line)
		if !ok {
			continue
		}
		target, ok := known[strings.ToLower(handle)]
		if !ok || target.Agent.ID == speakerAgentID {
			continue
		}
		if task == "" {
			task = "Please respond to the previous team handoff."
		}
		handoffs = append(handoffs, teamHandoff{
			Target:          target,
			Prompt:          task,
			SourceMessageID: sourceMessageID,
		})
	}
	return handoffs
}

func parseTeamHandoffLine(line string) (string, string, bool) {
	if !strings.HasPrefix(line, "@") {
		return "", "", false
	}
	end := 1
	for end < len(line) {
		ch := line[end]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			end++
			continue
		}
		break
	}
	if end == 1 {
		return "", "", false
	}
	if end < len(line) {
		ch := line[end]
		if ch != ' ' && ch != '\t' && ch != ':' && ch != '-' {
			return "", "", false
		}
	}
	task := strings.TrimSpace(line[end:])
	task = strings.TrimLeft(task, ":- \t")
	return line[1:end], strings.TrimSpace(task), true
}

func agentsByHandle(agents []ConversationAgentContext) map[string]ConversationAgentContext {
	known := make(map[string]ConversationAgentContext, len(agents))
	for _, agent := range agents {
		if agent.Agent.Handle != "" {
			known[strings.ToLower(agent.Agent.Handle)] = agent
		}
	}
	return known
}

type teamProtocolContextInput struct {
	RootMessage      domain.Message
	Roster           []ConversationAgentContext
	Leader           ConversationAgentContext
	Speaker          ConversationAgentContext
	Budget           teamBudget
	RunsUsed         int
	BatchesUsed      int
	SummaryMode      bool
	DiscussionDigest string
}

func teamProtocolContext(input teamProtocolContextInput) string {
	var b strings.Builder
	b.WriteString("AgentX team collaboration protocol for this turn.\n")
	fmt.Fprintf(&b, "Team leader: @%s (%s)\n", input.Leader.Agent.Handle, input.Leader.Agent.Name)
	fmt.Fprintf(&b, "You are: @%s (%s)\n", input.Speaker.Agent.Handle, input.Speaker.Agent.Name)
	fmt.Fprintf(&b, "Budget: %d/%d batches used, %d/%d agent runs used.\n", input.BatchesUsed, input.Budget.MaxBatches, input.RunsUsed, input.Budget.MaxRuns)
	b.WriteString("\nTeam roster:\n")
	for _, member := range input.Roster {
		description := strings.TrimSpace(member.Agent.Description)
		if description == "" {
			description = "No description configured."
		}
		fmt.Fprintf(&b, "- @%s (%s): %s\n", member.Agent.Handle, member.Agent.Name, description)
	}
	b.WriteString("\nOriginal user request:\n")
	b.WriteString(runtimeMessageBody(input.RootMessage.Body))
	b.WriteString("\n\nRules:\n")
	if input.SummaryMode {
		b.WriteString("- Produce the final answer for the user now.\n")
		b.WriteString("- Do not hand off to another agent and do not write @handle delegation lines.\n")
		b.WriteString("- Synthesize the team discussion, call out important disagreement, and give a concrete recommendation.\n")
		if digest := strings.TrimSpace(input.DiscussionDigest); digest != "" {
			b.WriteString("\nTeam discussion digest:\n")
			b.WriteString(digest)
			b.WriteByte('\n')
		}
		return strings.TrimSpace(b.String())
	}
	b.WriteString("- Answer the current task directly when you can.\n")
	b.WriteString("- Only involve another member when their configured responsibility is needed.\n")
	b.WriteString("- To involve a member, put each handoff on its own line starting with @handle followed by a concrete task.\n")
	b.WriteString("- Do not use @handle for casual mentions, acknowledgements, or final answers.\n")
	b.WriteString("- Do not hand off to yourself. Avoid repeating a handoff already answered in this discussion.\n")
	b.WriteString("- If budget is nearly exhausted, prefer a concise answer over another handoff.\n")
	return strings.TrimSpace(b.String())
}

func teamSummaryPrompt(rootMessage domain.Message, transcript []teamTranscriptEntry, stopReason string) string {
	var b strings.Builder
	b.WriteString("The team discussion is complete. Provide the final answer to the user.\n")
	fmt.Fprintf(&b, "Stop reason: %s\n\n", stopReason)
	b.WriteString("Original user request:\n")
	b.WriteString(runtimeMessageBody(rootMessage.Body))
	b.WriteString("\n\nTeam discussion:\n")
	b.WriteString(teamTranscriptText(transcript))
	b.WriteString("\n\nReturn a concise final answer. Do not write any @handle handoff lines.")
	return b.String()
}

func teamTranscriptText(transcript []teamTranscriptEntry) string {
	if len(transcript) == 0 {
		return "(no completed team messages)"
	}
	var b strings.Builder
	for _, entry := range transcript {
		fmt.Fprintf(&b, "@%s (%s):\n%s\n\n", entry.Handle, entry.Name, runtimeMessageBody(entry.Body))
	}
	return strings.TrimSpace(b.String())
}
