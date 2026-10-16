// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AgentEnvEditor, agentEnvRows, buildAgentEnvPatch } from "./AgentEnvEditor";

afterEach(cleanup);

const env = { API_KEY: "secret", EMPTY: "", HOME: "/home/agent" };

describe("buildAgentEnvPatch", () => {
  it("returns an empty patch when nothing changed", () => {
    expect(buildAgentEnvPatch(env, agentEnvRows(env))).toEqual({ patch: {} });
  });

  it("sets edited values, renames untouched keys, and unsets removed rows", () => {
    const [apiKey, empty, home] = agentEnvRows(env);
    apiKey.value = "new-secret";
    home.key = "USER_HOME";
    const next = [apiKey, home, { id: "new", key: " ADDED ", value: "1" }];
    expect(empty.originalKey).toBe("EMPTY");

    expect(buildAgentEnvPatch(env, next)).toEqual({
      patch: {
        set: { API_KEY: "new-secret", ADDED: "1" },
        unset: ["EMPTY"],
        rename: { HOME: "USER_HOME" },
      },
    });
  });

  it("treats a value edited back to the original as unchanged", () => {
    const [apiKey] = agentEnvRows({ API_KEY: "secret" });
    apiKey.value = "secret";
    expect(buildAgentEnvPatch({ API_KEY: "secret" }, [apiKey])).toEqual({ patch: {} });
  });

  it("replaces the key when both name and value changed", () => {
    const [apiKey] = agentEnvRows({ API_KEY: "secret" });
    apiKey.key = "TOKEN";
    apiKey.value = "v";
    expect(buildAgentEnvPatch({ API_KEY: "secret" }, [apiKey])).toEqual({
      patch: { set: { TOKEN: "v" }, unset: ["API_KEY"] },
    });
  });

  it("rejects missing, invalid, and duplicate names and skips blank new rows", () => {
    const blank = { id: "blank", key: "", value: "" };
    expect(buildAgentEnvPatch({}, [blank])).toEqual({ patch: {} });
    expect(buildAgentEnvPatch({}, [{ ...blank, value: "x" }]).error).toBe("Variable name is required");
    expect(buildAgentEnvPatch({}, [{ ...blank, key: "A=B" }]).error).toBe("Invalid variable name: A=B");
    const [home] = agentEnvRows({ HOME: "/home/agent" });
    expect(buildAgentEnvPatch({ HOME: "/home/agent" }, [home, { ...blank, key: "HOME" }]).error).toBe(
      "Duplicate variable: HOME"
    );
  });
});

describe("AgentEnvEditor", () => {
  it("shows values and saves only the changed entries", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<AgentEnvEditor env={env} onSave={onSave} />);

    expect((screen.getByLabelText("Value for API_KEY") as HTMLInputElement).value).toBe("secret");
    expect((screen.getByLabelText("Value for HOME") as HTMLInputElement).value).toBe("/home/agent");
    fireEvent.change(screen.getByLabelText("Value for API_KEY"), { target: { value: "rotated" } });
    fireEvent.click(screen.getByLabelText("Remove EMPTY"));
    fireEvent.click(screen.getByText("Add"));
    const names = screen.getAllByLabelText("Variable name");
    fireEvent.change(names[names.length - 1], { target: { value: "NEW_VAR" } });
    fireEvent.change(screen.getByLabelText("Value for NEW_VAR"), { target: { value: "x" } });
    fireEvent.click(screen.getByText("Save env"));

    await waitFor(() => expect(screen.getByText("Saved")).toBeTruthy());
    expect(onSave).toHaveBeenCalledWith({
      set: { API_KEY: "rotated", NEW_VAR: "x" },
      unset: ["EMPTY"],
    });
  });

  it("discards pending edits", () => {
    render(<AgentEnvEditor env={env} onSave={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Value for API_KEY"), { target: { value: "changed" } });
    fireEvent.click(screen.getByLabelText("Remove HOME"));
    expect(screen.queryByLabelText("Value for HOME")).toBeNull();
    fireEvent.click(screen.getByText("Discard"));
    expect((screen.getByLabelText("Value for HOME") as HTMLInputElement).value).toBe("/home/agent");
    expect((screen.getByLabelText("Value for API_KEY") as HTMLInputElement).value).toBe("secret");
  });
});
