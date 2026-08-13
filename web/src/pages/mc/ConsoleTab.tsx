// Live console tab: the shared ConsoleView plus a command input with
// history.

import { useRef, useState, type KeyboardEvent } from "react";
import { api } from "../../api/client";
import type { MCState } from "../../api/types";
import { useToast } from "../../ui/Toast";
import { ConsoleView } from "./ConsoleView";

export function ConsoleTab({ id, state }: { id: string; state: MCState }) {
  const [command, setCommand] = useState("");
  const [histIdx, setHistIdx] = useState(-1);
  const historyRef = useRef<string[]>([]);
  const toast = useToast();

  async function send() {
    const cmd = command.trim();
    if (!cmd) return;
    setCommand("");
    setHistIdx(-1);
    historyRef.current = [cmd, ...historyRef.current.filter((c) => c !== cmd)].slice(0, 50);
    try {
      await api(`/api/minecraft/${id}/command`, { method: "POST", body: { command: cmd } });
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "command failed");
    }
  }

  function onKey(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter") {
      void send();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      const next = Math.min(histIdx + 1, historyRef.current.length - 1);
      if (next >= 0 && historyRef.current[next] !== undefined) {
        setHistIdx(next);
        setCommand(historyRef.current[next]);
      }
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      const next = histIdx - 1;
      setHistIdx(next);
      setCommand(next >= 0 ? historyRef.current[next] : "");
    }
  }

  const canCommand = state === "running" || state === "starting";

  return (
    <div className="console-card">
      <ConsoleView id={id} />
      <div className="console-input">
        <span className="mono faint" style={{ alignSelf: "center" }}>
          &gt;
        </span>
        <input
          className="input"
          placeholder={canCommand ? "server command (e.g. list, say hello, tps)" : "server is not running"}
          value={command}
          disabled={!canCommand}
          onChange={(e) => setCommand(e.target.value)}
          onKeyDown={onKey}
          spellCheck={false}
          autoComplete="off"
        />
        <button className="btn" onClick={send} disabled={!canCommand || !command.trim()}>
          Send
        </button>
      </div>
    </div>
  );
}
