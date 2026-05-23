import { lazy, Suspense, useContext, useState } from "react";
import { cn } from "@/lib/utils";
import { AgentAvatar, agentKindColor } from "../AgentAvatar";
import type { PendingQuestion } from "../shell/types";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { MentionLabelsContext, displayMentionLabels, messageBodyClassName } from "./markdown";

const MarkdownRenderer = lazy(() =>
  import("../MarkdownRenderer").then((module) => ({ default: module.MarkdownRenderer }))
);

export function QuestionPrompt({
  question,
  agentName,
  agentKind,
  agentID,
  hideAvatar,
  onSubmit,
}: {
  question: PendingQuestion;
  agentName?: string;
  agentKind?: string;
  agentID?: string;
  hideAvatar?: boolean;
  onSubmit: (answer: string) => void;
}) {
  const [answer, setAnswer] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const mentionLabels = useContext(MentionLabelsContext);
  const label = agentName ?? "Agent";

  async function handleSubmit() {
    if (submitting) return;
    setSubmitting(true);
    try {
      onSubmit(answer);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="group flex min-w-0 max-w-full gap-3 rounded-md px-1 py-1 md:gap-4 md:px-2">
      {!hideAvatar && (
        agentID ? (
          <AgentAvatar agentID={agentID} kind={agentKind ?? "fake"} size="md" className="shrink-0" />
        ) : (
          <Avatar className="h-10 w-10 shrink-0">
            <AvatarFallback className={cn("text-white text-sm", agentKindColor(agentKind ?? "fake"))}>?</AvatarFallback>
          </Avatar>
        )
      )}

      <div className="min-w-0 flex-1 space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-semibold">{label}</span>
          <Badge variant="secondary" className="text-xs">BOT</Badge>
          <Badge variant="outline" className="text-xs text-amber-600 dark:text-amber-400 border-amber-300 dark:border-amber-600">QUESTION</Badge>
        </div>
        <div className={messageBodyClassName} data-testid="question-body">
          <Suspense fallback={<p>{displayMentionLabels(question.question, mentionLabels)}</p>}>
            <MarkdownRenderer text={question.question} mentionLabels={mentionLabels} />
          </Suspense>
        </div>
        {question.options && question.options.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {question.options.map((opt) => (
              <Button
                key={opt.label}
                variant={answer === opt.label ? "default" : "outline"}
                size="sm"
                onClick={() => setAnswer(opt.label)}
                disabled={submitting}
              >
                <span>{opt.label}</span>
                {opt.description && (
                  <span className="ml-1 text-muted-foreground text-xs">— {opt.description}</span>
                )}
              </Button>
            ))}
          </div>
        )}
        <div className="flex gap-2 items-end">
          <Textarea
            value={answer}
            onChange={(e) => setAnswer(e.target.value)}
            placeholder="Type your response..."
            className="min-h-[40px] max-h-[120px] resize-none text-sm"
            rows={1}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                handleSubmit();
              }
            }}
            disabled={submitting}
          />
          <Button
            onClick={handleSubmit}
            disabled={submitting}
            size="sm"
            className="shrink-0"
          >
            Send
          </Button>
        </div>
      </div>
    </div>
  );
}
