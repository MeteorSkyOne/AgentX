import { useEffect, useMemo, useState } from "react";
import { Plus, RotateCcw, Save, Trash2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { AgentEnvPatch } from "../../api/types";

export interface AgentEnvRow {
  id: string;
  originalKey?: string;
  originalValue?: string;
  key: string;
  value: string;
}

let nextRowID = 0;

function newRowID(): string {
  nextRowID += 1;
  return `env-row-${nextRowID}`;
}

export function agentEnvRows(env: Record<string, string>): AgentEnvRow[] {
  return Object.entries(env)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => ({
      id: newRowID(),
      originalKey: key,
      originalValue: value,
      key,
      value,
    }));
}

export function buildAgentEnvPatch(
  original: Record<string, string>,
  rows: AgentEnvRow[]
): { patch: AgentEnvPatch; error?: undefined } | { patch?: undefined; error: string } {
  const set: Record<string, string> = {};
  const unset: string[] = [];
  const rename: Record<string, string> = {};
  const seen = new Set<string>();
  const kept = new Set<string>();

  for (const row of rows) {
    const key = row.key.trim();
    if (!row.originalKey && !key && !row.value) continue;
    if (!key) return { error: "Variable name is required" };
    if (/[\s=]/.test(key)) return { error: `Invalid variable name: ${key}` };
    if (seen.has(key)) return { error: `Duplicate variable: ${key}` };
    seen.add(key);

    if (!row.originalKey) {
      set[key] = row.value;
      continue;
    }
    kept.add(row.originalKey);
    const valueEdited = row.value !== row.originalValue;
    if (key === row.originalKey) {
      if (valueEdited) set[key] = row.value;
    } else if (valueEdited) {
      unset.push(row.originalKey);
      set[key] = row.value;
    } else {
      rename[row.originalKey] = key;
    }
  }
  for (const key of Object.keys(original)) {
    if (!kept.has(key)) unset.push(key);
  }

  const patch: AgentEnvPatch = {};
  if (Object.keys(set).length > 0) patch.set = set;
  if (unset.length > 0) patch.unset = unset;
  if (Object.keys(rename).length > 0) patch.rename = rename;
  return { patch };
}

function rowChanged(row: AgentEnvRow): boolean {
  return !row.originalKey || row.value !== row.originalValue || row.key.trim() !== row.originalKey;
}

export function AgentEnvEditor({
  env,
  onSave,
}: {
  env: Record<string, string>;
  onSave: (patch: AgentEnvPatch) => Promise<void>;
}) {
  const envSignature = useMemo(
    () => JSON.stringify(Object.entries(env).sort(([left], [right]) => left.localeCompare(right))),
    [env]
  );
  const [rows, setRows] = useState<AgentEnvRow[]>(() => agentEnvRows(env));
  const [status, setStatus] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [savedCount, setSavedCount] = useState(0);

  // Rebuild rows when the server state changes or after a save.
  useEffect(() => {
    setRows(agentEnvRows(env));
  }, [envSignature, savedCount]);

  const removedCount = useMemo(() => {
    const kept = new Set(rows.map((row) => row.originalKey).filter(Boolean));
    return Object.keys(env).filter((key) => !kept.has(key)).length;
  }, [env, rows]);
  const dirty = removedCount > 0 || rows.some((row) => rowChanged(row) && (row.originalKey || row.key || row.value));

  function updateRow(id: string, change: Partial<AgentEnvRow>) {
    setStatus(null);
    setRows((current) => current.map((row) => (row.id === id ? { ...row, ...change } : row)));
  }

  function removeRow(id: string) {
    setStatus(null);
    setRows((current) => current.filter((row) => row.id !== id));
  }

  function addRow() {
    setStatus(null);
    setRows((current) => [
      ...current,
      { id: newRowID(), key: "", value: "" },
    ]);
  }

  function discard() {
    setRows(agentEnvRows(env));
    setStatus(null);
  }

  async function save() {
    const result = buildAgentEnvPatch(env, rows);
    if (result.error !== undefined) {
      setStatus(result.error);
      return;
    }
    if (Object.keys(result.patch).length === 0) {
      setStatus("No changes");
      return;
    }
    setSaving(true);
    try {
      await onSave(result.patch);
      setSavedCount((count) => count + 1);
      setStatus("Saved");
    } catch (err) {
      setStatus(err instanceof Error ? err.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground uppercase">Variables</span>
        <Button type="button" size="sm" variant="ghost" className="h-7 gap-1 px-2 text-xs" onClick={addRow}>
          <Plus className="h-3.5 w-3.5" />
          Add
        </Button>
      </div>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">No variables</p>
      ) : (
        <div className="space-y-2">
          {rows.map((row) => (
            <div
              key={row.id}
              className={cn(
                "flex items-center gap-1.5 rounded-lg border border-border p-1.5",
                rowChanged(row) && "border-primary/50 bg-accent/40"
              )}
            >
              <Input
                value={row.key}
                onChange={(e) => updateRow(row.id, { key: e.target.value })}
                placeholder="NAME"
                aria-label="Variable name"
                className="h-8 min-w-0 flex-1 font-mono text-xs"
                autoComplete="off"
                spellCheck={false}
              />
              <Input
                value={row.value}
                onChange={(e) => updateRow(row.id, { value: e.target.value })}
                placeholder="value"
                aria-label={`Value for ${row.key || "new variable"}`}
                className="h-8 min-w-0 flex-[1.4] font-mono text-xs"
                autoComplete="off"
                spellCheck={false}
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-8 w-8 shrink-0 text-muted-foreground hover:text-destructive"
                title="Remove variable"
                aria-label={`Remove ${row.key || "variable"}`}
                onClick={() => removeRow(row.id)}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>
          ))}
        </div>
      )}
      {removedCount > 0 && (
        <p className="text-xs text-muted-foreground">
          {removedCount} variable{removedCount === 1 ? "" : "s"} will be removed.
        </p>
      )}
      <div className="flex gap-2">
        <Button type="button" size="sm" variant="outline" className="flex-1 gap-2" onClick={discard} disabled={!dirty || saving}>
          <RotateCcw className="h-4 w-4" />
          Discard
        </Button>
        <Button type="button" size="sm" className="flex-1 gap-2" onClick={save} disabled={!dirty || saving}>
          <Save className="h-4 w-4" />
          Save env
        </Button>
      </div>
      {status && <p className="text-xs text-muted-foreground">{status}</p>}
    </div>
  );
}
