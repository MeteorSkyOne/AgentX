import { createContext, lazy, memo, Suspense, useContext } from "react";
import type { ConversationAgentContext } from "@/api/types";
import type { WorkspacePathTarget } from "@/lib/workspacePaths";
import type { MentionLabels } from "../MarkdownRenderer";

const MarkdownRenderer = lazy(() =>
  import("../MarkdownRenderer").then((module) => ({ default: module.MarkdownRenderer }))
);

export const messageBodyClassName =
  "prose prose-sm min-w-0 w-full max-w-full overflow-x-auto break-words select-text dark:prose-invert";
export const MentionLabelsContext = createContext<MentionLabels | undefined>(undefined);
const MENTION_TEXT_RE = /@([A-Za-z0-9][A-Za-z0-9_-]*)/g;

export function buildMentionLabels(agents: ConversationAgentContext[]): MentionLabels {
  const labels: MentionLabels = {};
  for (const { agent } of agents) {
    if (agent.handle && agent.name) {
      labels[agent.handle.toLowerCase()] = agent.name;
    }
  }
  return labels;
}

export function displayMentionLabels(text: string, mentionLabels?: MentionLabels): string {
  if (!mentionLabels) return text;
  return text.replace(MENTION_TEXT_RE, (match, handle: string) => {
    const label = mentionLabels[handle.toLowerCase()]?.trim();
    return label ? `@${label}` : match;
  });
}

export const MessageMarkdown = memo(function MessageMarkdown({
  text,
  workspacePath,
  onOpenWorkspacePath,
}: {
  text: string;
  workspacePath?: string;
  onOpenWorkspacePath?: (target: WorkspacePathTarget) => void;
}) {
  const mentionLabels = useContext(MentionLabelsContext);

  return (
    <Suspense fallback={<MarkdownFallback text={text} />}>
      <MarkdownRenderer
        text={text}
        workspacePath={workspacePath}
        onOpenWorkspacePath={onOpenWorkspacePath}
        mentionLabels={mentionLabels}
      />
    </Suspense>
  );
});

export function MarkdownFallback({ text }: { text: string }) {
  const mentionLabels = useContext(MentionLabelsContext);
  return <p className="whitespace-pre-wrap">{displayMentionLabels(text, mentionLabels)}</p>;
}
