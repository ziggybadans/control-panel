// Live console: backlog fetch + SSE line stream, autoscroll with scroll
// lock, command input with history.

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import { api } from "../../api/client";
import type { LogLine, MCState } from "../../api/types";
import { fmtTime } from "../../lib/format";
import { Icon } from "../../ui/Icon";
import { useToast } from "../../ui/Toast";

const MAX_LINES = 2000;

export function ConsoleTab({ id, state }: { id: string; state: MCState }) {
  const [lines, setLines] = useState<LogLine[]>([]);
  const [command, setCommand] = useState("");
  const [histIdx, setHistIdx] = useState(-1);
  const [pinned, setPinned] = useState(true);
  const historyRef = useRef<string[]>([]);
  const scrollRef = useRef<HTMLDivElement>(null);
  const toast = useToast();

  // Backlog + live stream.
  useEffect(() => {
    let closed = false;
    setLines([]);
    api<{ lines: LogLine[] }>(`/api/minecraft/${id}/console?tail=500`)
      .then((res) => {
        if (!closed) setLines(res.lines ?? []);
      })
      .catch(() => {});
    const es = new EventSource(`/api/minecraft/${id}/console/stream`);
    es.addEventListener("line", (e) => {
      const line: LogLine = JSON.parse((e as MessageEvent).data);
      setLines((prev) => {
        const next = prev.length >= MAX_LINES ? prev.slice(-MAX_LINES + 1) : prev.slice();
        next.push(line);
        return next;
      });
    });
    return () => {
      closed = true;
      es.close();
    };
  }, [id]);

  // Autoscroll while pinned to the bottom.
  useEffect(() => {
    if (pinned && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [lines, pinned]);

  const onScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    setPinned(el.scrollHeight - el.scrollTop - el.clientHeight < 40);
  }, []);

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
      <div ref={scrollRef} className="console" onScroll={onScroll}>
        {lines.length === 0 && (
          <div className="ln" style={{ color: "var(--text-3)" }}>
            — no console output yet —
          </div>
        )}
        {lines.map((l, i) => (
          <div key={i} className={`ln ${l.level.toLowerCase()}`}>
            {/* Server log lines carry their own [HH:MM:SS]; only prefix ours
                on panel/command lines that don't. */}
            {!/^\[\d\d:\d\d:\d\d\]/.test(l.text) && (
              <span style={{ opacity: 0.45 }}>{fmtTime(l.ts)} </span>
            )}
            {l.text}
          </div>
        ))}
      </div>
      {!pinned && (
        <button
          className="btn btn-sm"
          style={{ position: "absolute", right: 24, bottom: 64 }}
          onClick={() => {
            setPinned(true);
            scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
          }}
        >
          <Icon name="chevronDown" size={12} />
          Follow
        </button>
      )}
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
