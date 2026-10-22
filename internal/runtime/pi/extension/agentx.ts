/**
 * AgentX bridge extension for pi.
 *
 * AgentX writes this file next to its data directory and loads it into every
 * pi process it starts (`pi --mode rpc -e <path>`). It fills the gaps between
 * what AgentX offers for Claude Code / Codex agents and what pi ships with:
 *
 * - `ask_user` tool: lets the model ask the user a question. In RPC mode the
 *   dialog becomes an `extension_ui_request`, which AgentX renders as a
 *   question prompt and answers with `extension_ui_response`.
 * - `update_plan` tool: lets the model publish its step list. AgentX reads the
 *   tool arguments and shows them as run progress.
 * - `/agentx-mode plan=on|off yolo=on|off`: AgentX sends this before a turn
 *   when the agent's settings changed. Plan mode blocks edit/write and any
 *   non read-only shell command; yolo off asks before dangerous shell commands.
 */

import { StringEnum, Type } from "@earendil-works/pi-ai";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

interface Mode {
	plan: boolean;
	yolo: boolean;
}

const mode: Mode = { plan: false, yolo: true };

const WRITE_TOOLS = new Set(["edit", "write"]);

// Commands that get a confirmation when yolo mode is off.
const DANGEROUS_PATTERNS = [
	/\brm\s+(-[a-zA-Z]*[rRf][a-zA-Z]*\s+|--recursive|--force)/,
	/\bsudo\b/,
	/\bsu\s+-?\b/,
	/\b(chmod|chown|chgrp)\b.*\b(777|-R|--recursive)\b/,
	/\bdd\s+if=/,
	/\bmkfs(\.\w+)?\b/,
	/>\s*\/dev\/(sd|nvme|hd|disk)/,
	/\bgit\s+push\b.*(--force|-f\b|\+[^\s]+:)/,
	/\bgit\s+(reset\s+--hard|clean\s+-[a-zA-Z]*f|checkout\s+--\s+\.|branch\s+-D)/,
	/\bgit\s+push\b.*--delete/,
	/\b(shutdown|reboot|halt|poweroff)\b/,
	/\bkill\s+-9\s+-1\b|\bkillall\b|\bpkill\b/,
	/\b(truncate|shred)\b/,
	/\bdocker\s+(system\s+prune|rm\s+-f|rmi|volume\s+rm)/,
	/\b(DROP|TRUNCATE)\s+(TABLE|DATABASE|SCHEMA)\b/i,
	/\bcurl\b.*\|\s*(ba|z|k)?sh\b/,
	/\bwget\b.*\|\s*(ba|z|k)?sh\b/,
];

// Plan mode: anything matching these is never read-only.
const PLAN_DESTRUCTIVE_PATTERNS = [
	/\brm\b/,
	/\brmdir\b/,
	/\bmv\b/,
	/\bcp\b/,
	/\bmkdir\b/,
	/\btouch\b/,
	/\bchmod\b/,
	/\bchown\b/,
	/\bln\b/,
	/\btee\b/,
	/\btruncate\b/,
	/\bdd\b/,
	/(^|[^<>])>(?!>)/,
	/>>/,
	/\b(npm|pnpm|yarn|bun)\s+(install|add|remove|uninstall|update|ci|link|publish)\b/,
	/\bpip3?\s+(install|uninstall)\b/,
	/\bgo\s+(install|get|mod\s+tidy)\b/,
	/\bcargo\s+(install|add|remove)\b/,
	/\bgit\s+(add|commit|push|pull|merge|rebase|reset|checkout|switch|restore|stash|cherry-pick|revert|tag|init|clone|branch\s+-[dDm])/,
	/\bsudo\b/,
	/\bkill\b|\bpkill\b|\bkillall\b/,
	/\bsystemctl\s+(start|stop|restart|enable|disable)/,
	/\b(vim?|nano|emacs)\b/,
];

// Plan mode: read-only commands that may run.
const PLAN_SAFE_PATTERNS = [
	/^\s*(cat|head|tail|less|more|wc|sort|uniq|cut|tr|diff|file|stat|du|df|tree|which|whereis|type|env|printenv|uname|whoami|id|date|pwd|echo|printf|ls|find|grep|rg|fd|bat|eza|jq|yq|awk|xargs|basename|dirname|realpath|readlink|od|hexdump|strings|column|nl|tac|rev|comm|md5sum|sha\w*sum)\b/,
	/^\s*sed\s+-n\b/,
	/^\s*git\s+(status|log|diff|show|branch|remote|blame|describe|rev-parse|ls-files|ls-tree|cat-file|shortlog|stash\s+list|config\s+--get|worktree\s+list)\b/,
	/^\s*(npm|pnpm|yarn)\s+(list|ls|view|info|why|outdated|audit|run\s+(test|lint|typecheck|check|build))\b/,
	/^\s*go\s+(list|vet|test|build|doc|version|env)\b/,
	/^\s*cargo\s+(check|test|build|metadata|tree)\b/,
	/^\s*(python3?|node|ruby|java|rustc|tsc|make)\s+(--version|-v|-V)\b/,
	/^\s*make\s+(test|check|lint)\b/,
	/^\s*(ps|top|free|uptime)\b/,
	/^\s*curl\s/,
	/^\s*wget\s+-O\s*-/,
];

function isDangerousCommand(command: string): boolean {
	return DANGEROUS_PATTERNS.some((pattern) => pattern.test(command));
}

function isPlanSafeCommand(command: string): boolean {
	if (PLAN_DESTRUCTIVE_PATTERNS.some((pattern) => pattern.test(command))) return false;
	// Every pipeline segment / chained command has to be allowlisted.
	const segments = command.split(/\|\||&&|;|\|/).map((segment) => segment.trim()).filter(Boolean);
	if (segments.length === 0) return false;
	return segments.every((segment) => PLAN_SAFE_PATTERNS.some((pattern) => pattern.test(segment)));
}

function parseMode(args: string): Partial<Mode> {
	const next: Partial<Mode> = {};
	for (const token of args.split(/\s+/).filter(Boolean)) {
		const [key, rawValue] = token.split("=");
		const value = (rawValue ?? "on").toLowerCase();
		const enabled = value === "on" || value === "true" || value === "1" || value === "yes";
		if (key === "plan") next.plan = enabled;
		if (key === "yolo") next.yolo = enabled;
	}
	return next;
}

const AskUserParams = Type.Object({
	question: Type.String({ description: "The question to ask the user. Be specific and include the context they need." }),
	options: Type.Optional(
		Type.Array(Type.String(), {
			description: "Optional list of short answer choices. Omit it to request free-form text.",
		}),
	),
});

const PlanStepStatus = StringEnum(["pending", "in_progress", "completed"] as const);

const UpdatePlanParams = Type.Object({
	explanation: Type.Optional(Type.String({ description: "One sentence on why the plan changed." })),
	plan: Type.Array(
		Type.Object({
			step: Type.String({ description: "Short description of the step." }),
			status: PlanStepStatus,
		}),
		{ description: "The complete current step list, in order. At most one step should be in_progress." },
	),
});

export default function (pi: ExtensionAPI) {
	pi.registerCommand("agentx-mode", {
		description: "AgentX internal: set plan/yolo mode (plan=on|off yolo=on|off)",
		handler: async (args) => {
			Object.assign(mode, parseMode(args ?? ""));
		},
	});

	pi.registerTool({
		name: "ask_user",
		label: "Ask user",
		description:
			"Ask the user a question and wait for the answer. Use it when you need a decision, missing information, or approval before continuing. Prefer offering a few concrete options; the user can always answer with their own text.",
		parameters: AskUserParams,
		exposure: "model-only",
		async execute(_toolCallId, params, _signal, _onUpdate, ctx) {
			if (!ctx.hasUI) {
				return {
					content: [{ type: "text", text: "No user interface is available to answer the question. Proceed with your best judgment and state the assumption." }],
					details: { question: params.question, options: params.options ?? [], answer: null, cancelled: true },
				};
			}
			const options = (params.options ?? []).map((option) => option.trim()).filter(Boolean);
			const answer = options.length > 0 ? await ctx.ui.select(params.question, options) : await ctx.ui.input(params.question);
			if (answer === undefined || answer === "") {
				return {
					content: [{ type: "text", text: "The user dismissed the question without answering. Proceed with your best judgment and state the assumption." }],
					details: { question: params.question, options, answer: null, cancelled: true },
				};
			}
			return {
				content: [{ type: "text", text: `User answered: ${answer}` }],
				details: { question: params.question, options, answer, cancelled: false },
			};
		},
	});

	pi.registerTool({
		name: "update_plan",
		label: "Update plan",
		description:
			"Publish or update your step-by-step plan for the current task so the user can follow progress. Call it when you start a multi-step task, and again whenever a step changes status. Always send the complete list.",
		parameters: UpdatePlanParams,
		exposure: "model-only",
		async execute(_toolCallId, params) {
			const lines = params.plan.map((item) => {
				const marker = item.status === "completed" ? "[x]" : item.status === "in_progress" ? "[>]" : "[ ]";
				return `${marker} ${item.step}`;
			});
			return {
				content: [{ type: "text", text: lines.length > 0 ? `Plan updated:\n${lines.join("\n")}` : "Plan cleared." }],
				details: { explanation: params.explanation ?? "", plan: params.plan },
			};
		},
	});

	pi.on("tool_call", async (event, ctx) => {
		const toolName = event.toolName;
		const input = event.input as Record<string, unknown>;
		const command = toolName === "bash" && typeof input.command === "string" ? input.command : undefined;

		if (mode.plan) {
			if (WRITE_TOOLS.has(toolName)) {
				return { block: true, reason: `Plan mode: ${toolName} is disabled. Describe the change in the plan instead of applying it.` };
			}
			if (command !== undefined && !isPlanSafeCommand(command)) {
				return { block: true, reason: `Plan mode: only read-only commands are allowed. Blocked: ${command}` };
			}
		}

		if (!mode.yolo && command !== undefined && isDangerousCommand(command)) {
			if (!ctx.hasUI) {
				return { block: true, reason: "Dangerous command blocked: no user available to approve it." };
			}
			const choice = await ctx.ui.select(`Allow this command?\n\n${command}`, ["Allow", "Block"]);
			if (choice !== "Allow") {
				return { block: true, reason: `The user blocked this command: ${command}` };
			}
		}
		return undefined;
	});
}
