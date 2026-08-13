// Web terminal: xterm.js bound to a server PTY session over SSE (output)
// and POSTs (input/resize). Opening a session is the panel's most powerful
// action, so it sits behind a typed confirmation and is audited.

import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { api } from "../api/client";
import type { TerminalSession, TerminalStatus } from "../api/types";
import { fmtTime } from "../lib/format";
import { EmptyState, Spinner } from "../ui/bits";
import { useConfirm } from "../ui/Confirm";
import { Icon } from "../ui/Icon";
import { useToast } from "../ui/Toast";

function b64encode(s: string): string {
  const bytes = new TextEncoder().encode(s);
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin);
}

function b64decode(b: string): Uint8Array {
  const bin = atob(b);
  const arr = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i);
  return arr;
}

// Default export so the router can lazy-load this page: xterm.js is the
// heaviest dependency in the bundle and only terminal users need it.
export default function TerminalPage() {
  const confirm = useConfirm();
  const toast = useToast();
  const [active, setActive] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const { data, refetch } = useQuery({
    queryKey: ["terminal"],
    queryFn: () => api<TerminalStatus>("/api/terminal"),
    refetchInterval: 15_000,
  });

  if (!data) {
    return (
      <div className="page">
        <Spinner />
      </div>
    );
  }
  if (!data.enabled) {
    return (
      <div className="page">
        <EmptyState
          icon="terminal"
          title="Terminal is disabled"
          hint={
            <>
              A web terminal is a real shell — the one exception to the panel's
              allowlist-only execution model — so it must be switched on
              deliberately: set <span className="mono">terminal.enabled: true</span>{" "}
              and a <span className="mono">terminal.run_as</span> user in
              config.yaml, then restart the panel.
            </>
          }
        />
      </div>
    );
  }

  const sessions = data.sessions ?? [];
  const activeId = sessions.some((s) => s.id === active) ? active : sessions[0]?.id ?? null;

  async function openSession() {
    const ok = await confirm({
      title: "Open terminal session",
      target: "terminal",
      typed: true,
      body: (
        <>
          This opens a real shell on the server ({data?.description}). Anything
          you run in it happens outside the panel's allowlist and confirmation
          checks. Session open/close is recorded in the audit log; idle
          sessions close automatically.
        </>
      ),
      confirmLabel: "Open shell",
    });
    if (!ok) return;
    setBusy(true);
    try {
      const v = await api<TerminalSession>("/api/terminal", {
        method: "POST",
        body: { cols: 120, rows: 32 },
        confirm: "terminal",
      });
      await refetch();
      setActive(v.id);
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "failed to open session");
    } finally {
      setBusy(false);
    }
  }

  async function closeSession(id: string) {
    try {
      await api(`/api/terminal/${id}`, { method: "DELETE" });
    } catch {
      // already gone is fine
    }
    setActive(null);
    await refetch();
  }

  return (
    <div className="page term-page">
      <div className="row wrap">
        <div className="choice-row" role="tablist" aria-label="terminal sessions">
          {sessions.map((s, i) => (
            <button
              key={s.id}
              role="tab"
              aria-selected={s.id === activeId}
              className={`choice ${s.id === activeId ? "selected" : ""}`}
              onClick={() => setActive(s.id)}
              title={`opened ${fmtTime(s.startedAt)}`}
            >
              <span className="mono">{i + 1}: {s.id.slice(0, 6)}</span>
            </button>
          ))}
        </div>
        <span className="small muted">{data.description}</span>
        <div className="row right">
          {activeId && (
            <button
              className="btn btn-sm"
              onClick={() => void closeSession(activeId)}
              title="End this shell session"
            >
              <Icon name="x" size={12} />
              Close session
            </button>
          )}
          <button
            className="btn btn-sm btn-primary"
            disabled={busy || sessions.length >= (data.maxSessions ?? 2)}
            onClick={() => void openSession()}
          >
            {busy ? <Spinner size={11} /> : <Icon name="terminal" size={12} />}
            New session
          </button>
        </div>
      </div>

      {activeId ? (
        <TerminalView key={activeId} id={activeId} onExited={() => void refetch()} />
      ) : (
        <EmptyState
          icon="terminal"
          title="No open sessions"
          hint="Open a session to get a shell. Sessions are audited and close when idle."
        />
      )}
    </div>
  );
}

function TerminalView({ id, onExited }: { id: string; onExited: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const exitedRef = useRef(onExited);
  exitedRef.current = onExited;

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const styles = getComputedStyle(document.documentElement);
    const term = new XTerm({
      fontFamily: styles.getPropertyValue("--font-mono").trim() || "ui-monospace, monospace",
      fontSize: 12.5,
      lineHeight: 1.25,
      cursorBlink: true,
      scrollback: 5000,
      theme: {
        background: styles.getPropertyValue("--console-bg").trim() || "#10151c",
        foreground: "#c9d2de",
        cursor: "#c9d2de",
        selectionBackground: "#3a4657",
      },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(el);
    fit.fit();

    let lastCols = term.cols;
    let lastRows = term.rows;
    const resize = () =>
      api(`/api/terminal/${id}/resize`, {
        method: "POST",
        body: { cols: term.cols, rows: term.rows },
      }).catch(() => {});
    void resize();

    // Input goes over a serialized promise chain so keystroke order is
    // preserved even when POSTs overlap.
    let chain: Promise<unknown> = Promise.resolve();
    const disp = term.onData((input) => {
      chain = chain
        .then(() =>
          api(`/api/terminal/${id}/input`, { method: "POST", body: { b64: b64encode(input) } }),
        )
        .catch(() => {});
    });

    const es = new EventSource(`/api/terminal/${id}/stream`);
    es.addEventListener("data", (e) => {
      const { b64 } = JSON.parse((e as MessageEvent).data) as { b64: string };
      term.write(b64decode(b64));
    });
    es.addEventListener("exit", () => {
      term.write("\r\n\x1b[2m— session ended —\x1b[0m\r\n");
      es.close();
      exitedRef.current();
    });

    const ro = new ResizeObserver(() => {
      fit.fit();
      if (term.cols !== lastCols || term.rows !== lastRows) {
        lastCols = term.cols;
        lastRows = term.rows;
        void resize();
      }
    });
    ro.observe(el);
    term.focus();

    return () => {
      ro.disconnect();
      es.close();
      disp.dispose();
      term.dispose();
    };
  }, [id]);

  return <div ref={ref} className="term-container" />;
}
