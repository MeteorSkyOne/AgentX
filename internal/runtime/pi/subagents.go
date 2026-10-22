package pi

import (
	"fmt"
	"sort"

	"github.com/meteorsky/agentx/internal/runtime"
)

// subagentToolName is the delegation tool registered by the pi-subagents
// package (https://pi.dev/packages/pi-subagents). pi itself ships no subagents.
const subagentToolName = "subagent"

// subagentTracker turns `subagent` tool executions into the subagent_started /
// subagent_completed signals the UI uses, and surfaces the child's tool
// activity (reported through tool_execution_update partial results) as nested
// items under the spawning call. Foreground children run inside the parent pi
// process and never emit their own tool_execution events, so the progress
// snapshot is the only view the host gets.
type subagentTracker struct {
	runs map[string]*subagentRun
}

type subagentRun struct {
	// lastToolEnd remembers, per child index, the end timestamp of the latest
	// recentTools entry already surfaced so each child tool shows up once.
	lastToolEnd map[int64]int64
}

func newSubagentTracker() *subagentTracker {
	return &subagentTracker{runs: map[string]*subagentRun{}}
}

func isSubagentStart(rec record) bool {
	return stringVal(rec, "toolName") == subagentToolName && stringVal(rec, "parentToolCallId") == ""
}

func (t *subagentTracker) started(rec record) []runtime.ProcessItem {
	if !isSubagentStart(rec) {
		return nil
	}
	callID := stringVal(rec, "toolCallId")
	if callID == "" {
		return nil
	}
	if _, exists := t.runs[callID]; exists {
		return nil
	}
	t.runs[callID] = &subagentRun{lastToolEnd: map[int64]int64{}}
	return []runtime.ProcessItem{{Type: "subagent_started", ToolCallID: callID}}
}

func (t *subagentTracker) updated(rec record) []runtime.ProcessItem {
	callID := stringVal(rec, "toolCallId")
	run := t.runs[callID]
	if run == nil {
		return nil
	}
	details := mapVal(mapVal(rec, "partialResult"), "details")
	var items []runtime.ProcessItem
	for _, raw := range sliceVal(details, "progress") {
		progress, _ := raw.(record)
		if progress == nil {
			continue
		}
		items = append(items, run.childToolItems(callID, progress)...)
	}
	return items
}

// childToolItems returns one completed tool_call item per child tool not yet
// surfaced, in execution order.
func (r *subagentRun) childToolItems(callID string, progress record) []runtime.ProcessItem {
	index := int64Val(progress, "index")
	recent := sliceVal(progress, "recentTools")
	type childTool struct {
		tool  string
		args  string
		endMs int64
	}
	var fresh []childTool
	for _, raw := range recent {
		entry, _ := raw.(record)
		if entry == nil {
			continue
		}
		endMs := int64Val(entry, "endMs")
		if endMs <= r.lastToolEnd[index] {
			continue
		}
		fresh = append(fresh, childTool{tool: stringVal(entry, "tool"), args: stringVal(entry, "args"), endMs: endMs})
	}
	if len(fresh) == 0 {
		return nil
	}
	sort.Slice(fresh, func(i, j int) bool { return fresh[i].endMs < fresh[j].endMs })
	agent := stringVal(progress, "agent")
	items := make([]runtime.ProcessItem, 0, len(fresh))
	for _, tool := range fresh {
		if tool.endMs > r.lastToolEnd[index] {
			r.lastToolEnd[index] = tool.endMs
		}
		var input any
		if tool.args != "" {
			input = map[string]any{"args": tool.args}
		}
		items = append(items, runtime.ProcessItem{
			Type:             "tool_call",
			ToolName:         tool.tool,
			ToolCallID:       fmt.Sprintf("%s/%d/%d", callID, index, tool.endMs),
			Status:           "completed",
			Input:            input,
			Raw:              map[string]any{"agent": agent, "tool": tool.tool, "args": tool.args, "endMs": tool.endMs},
			ParentToolCallID: callID,
		})
	}
	return items
}

func (t *subagentTracker) ended(rec record) []runtime.ProcessItem {
	callID := stringVal(rec, "toolCallId")
	if _, ok := t.runs[callID]; !ok {
		return nil
	}
	delete(t.runs, callID)
	status := "completed"
	if boolVal(rec, "isError") {
		status = "failed"
	} else {
		details := mapVal(mapVal(rec, "result"), "details")
		for _, raw := range sliceVal(details, "results") {
			result, _ := raw.(record)
			if result == nil {
				continue
			}
			if int64Val(result, "exitCode") != 0 || stringVal(result, "error") != "" {
				status = "failed"
				break
			}
			if boolVal(result, "stopped") || boolVal(result, "interrupted") {
				status = "stopped"
			}
		}
	}
	return []runtime.ProcessItem{{Type: "subagent_completed", ToolCallID: callID, Status: status}}
}
