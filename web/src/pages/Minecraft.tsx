// Minecraft server list: one row per server — the card with live vitals and
// quick lifecycle actions, and its live console alongside (read-only here;
// commands are sent from the server's Console tab).

import { useState } from "react";
import { Link } from "react-router-dom";
import { useMCServers } from "../state/live";
import type { MCServer } from "../api/types";
import { fmtBytes, fmtPct, fmtUptimeSince } from "../lib/format";
import { TopbarActions } from "../shell/Layout";
import { Card, EmptyState, MCStateBadge } from "../ui/bits";
import { Icon } from "../ui/Icon";
import { useMCActions } from "./mc/actions";
import { ConsoleView } from "./mc/ConsoleView";
import { ServerListEntry } from "./mc/Motd";
import { SetupModal } from "./mc/SetupModal";

export function MinecraftPage() {
  const servers = useMCServers();
  const [setupOpen, setSetupOpen] = useState(false);

  return (
    <div className="page">
      <TopbarActions>
        <span className="small muted num">
          {servers.length} server{servers.length === 1 ? "" : "s"}
        </span>
        <button className="btn btn-sm btn-primary" onClick={() => setSetupOpen(true)}>
          <Icon name="plus" size={12} />
          New server
        </button>
      </TopbarActions>
      {setupOpen && <SetupModal onClose={() => setSetupOpen(false)} />}
      {servers.length === 0 ? (
        <Card title="Servers">
          <EmptyState
            icon="minecraft"
            title="No servers discovered"
            hint={
              <>
                Each directory under <span className="mono">minecraft.root</span> containing
                a server jar (or run.sh) becomes a managed server.
              </>
            }
          />
        </Card>
      ) : (
        <div className="mc-rows">
          {servers.map((s) => (
            <div key={s.id} className="mc-row">
              <ServerCard server={s} />
              <ConsoleView
                id={s.id}
                tail={100}
                // Streams only for active servers so many stopped servers
                // don't exhaust the browser's per-host connections.
                live={s.state !== "stopped" && s.state !== "crashed"}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function ServerCard({ server: s }: { server: MCServer }) {
  const { busy, start, stop, restart } = useMCActions(s.id);
  const players = s.onlinePlayers ?? [];
  const canStart = s.state === "stopped" || s.state === "crashed";
  const running = s.state === "running";

  return (
    <div className="card">
      <div className="card-h">
        <Icon name="minecraft" size={15} className="faint" />
        <Link to={`/minecraft/${s.id}`} style={{ color: "inherit", fontWeight: 650 }}>
          {s.name}
        </Link>
        <span className="small faint">
          {s.software} {s.version}
        </span>
        <div className="row right">
          <MCStateBadge state={s.state} />
        </div>
      </div>
      <div className="card-b" style={{ display: "flex", flexDirection: "column", gap: 10 }}>
        <ServerListEntry icon={s.icon} motd={s.motd} />
        <dl className="kv">
          <dt>Players</dt>
          <dd className="num">
            {running ? `${players.length} / ${s.maxPlayers ?? "—"}` : "—"}
            {players.length > 0 && (
              <span className="faint"> · {players.join(", ")}</span>
            )}
          </dd>
          <dt>Port</dt>
          <dd className="num">{s.port || "—"}</dd>
          <dt>Uptime</dt>
          <dd className="num">{running || s.state === "starting" ? fmtUptimeSince(s.startedAt) : "—"}</dd>
          <dt>CPU · Memory</dt>
          <dd className="num">
            {running
              ? `${fmtPct(s.cpuPct)} · ${fmtBytes(s.memBytes)} / ${fmtBytes(s.memMax)}`
              : "—"}
          </dd>
        </dl>
        {s.lastExit && s.state !== "running" && (
          <div className={`small ${s.state === "crashed" ? "crit-text" : "faint"}`}>
            {s.lastExit}
          </div>
        )}
        {!s.eulaAccepted && (
          <div className="small warn-text">
            <Icon name="warning" size={12} /> EULA not accepted — open settings
          </div>
        )}
        <div className="row" style={{ marginTop: "auto" }}>
          {canStart ? (
            <button
              className="btn btn-sm btn-primary"
              disabled={busy !== null || !s.eulaAccepted}
              onClick={start}
            >
              <Icon name="play" size={12} />
              Start
            </button>
          ) : (
            <>
              <button className="btn btn-sm" disabled={busy !== null || !running} onClick={restart}>
                <Icon name="restart" size={12} />
                Restart
              </button>
              <button className="btn btn-sm btn-danger" disabled={busy !== null || !running} onClick={stop}>
                <Icon name="stop" size={12} />
                Stop
              </button>
            </>
          )}
          <Link to={`/minecraft/${s.id}`} className="btn btn-sm btn-ghost right">
            Manage →
          </Link>
        </div>
      </div>
    </div>
  );
}
