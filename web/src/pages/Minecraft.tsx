// Minecraft server list: one card per server with live vitals and quick
// lifecycle actions.

import { Link } from "react-router-dom";
import { useMCServers } from "../state/live";
import type { MCServer } from "../api/types";
import { fmtBytes, fmtPct, fmtUptimeSince } from "../lib/format";
import { Card, EmptyState, MCStateBadge } from "../ui/bits";
import { Icon } from "../ui/Icon";
import { useMCActions } from "./mc/actions";

export function MinecraftPage() {
  const servers = useMCServers();

  return (
    <div className="page">
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
        <div className="grid" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(340px, 1fr))" }}>
          {servers.map((s) => (
            <ServerCard key={s.id} server={s} />
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
            <Icon name="terminal" size={12} />
            Console
          </Link>
        </div>
      </div>
    </div>
  );
}
