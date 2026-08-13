// Read-only live console: backlog fetch + SSE line stream with autoscroll
// and a follow button. ConsoleTab adds the command input on top of this;
// the Minecraft overview embeds it directly next to each server card.

import { useCallback, useEffect, useRef, useState, type CSSProperties } from "react";
import { api } from "../../api/client";
import type { LogLine } from "../../api/types";
import { fmtTime } from "../../lib/format";
import { Icon } from "../../ui/Icon";

const MAX_LINES = 2000;

export function ConsoleView({
  id,
  tail = 500,
  live = true,
  style,
}: {
  id: string;
  tail?: number;
  /** Subscribe to the live stream (off for stopped servers so a page of
   *  many servers doesn't exhaust the browser's per-host connections). */
  live?: boolean;
  style?: CSSProperties;
}) {
  const [lines, setLines] = useState<LogLine[]>([]);
  const [pinned, setPinned] = useState(true);
  const scrollRef = useRef<HTMLDivElement>(null);

  // Backlog + (optionally) the live stream.
  useEffect(() => {
    let closed = false;
    setLines([]);
    api<{ lines: LogLine[] }>(`/api/minecraft/${id}/console?tail=${tail}`)
      .then((res) => {
        if (!closed) setLines(res.lines ?? []);
      })
      .catch(() => {});
    if (!live) {
      return () => {
        closed = true;
      };
    }
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
  }, [id, tail, live]);

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

  return (
    <div className="console-view" style={style}>
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
          className="btn btn-sm fade-in-up console-follow"
          onClick={() => {
            setPinned(true);
            scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
          }}
        >
          <Icon name="chevronDown" size={12} />
          Follow
        </button>
      )}
    </div>
  );
}
