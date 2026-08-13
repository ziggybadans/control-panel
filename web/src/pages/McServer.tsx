// Minecraft server detail: header with lifecycle controls + tabbed console,
// players, backups, and settings.

import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMCServer } from "../state/live";
import { fmtBytes, fmtPct, fmtUptimeSince } from "../lib/format";
import { Card, EmptyState, MCStateBadge, Spinner } from "../ui/bits";
import { Icon } from "../ui/Icon";
import { withViewTransition } from "../ui/motion";
import { useMCActions } from "./mc/actions";
import { ConsoleTab } from "./mc/ConsoleTab";
import { PlayersTab } from "./mc/PlayersTab";
import { FilesTab } from "./mc/FilesTab";
import { AddonsTab } from "./mc/AddonsTab";
import { BackupsTab } from "./mc/BackupsTab";
import { TasksTab } from "./mc/TasksTab";
import { MapTab } from "./mc/MapTab";
import { SettingsTab } from "./mc/SettingsTab";

const TABS = ["Console", "Players", "Files", "Addons", "Backups", "Tasks", "Map", "Settings"] as const;
type Tab = (typeof TABS)[number];

export function MCServerPage() {
  const { id = "" } = useParams();
  const server = useMCServer(id);
  const { busy, start, stop, restart, kill } = useMCActions(id);
  const [tab, setTab] = useState<Tab>("Console");

  if (!server) {
    return (
      <div className="page">
        <Card>
          <EmptyState
            icon="minecraft"
            title={`Server “${id}” not found`}
            hint={<Link to="/minecraft">Back to server list</Link>}
          />
        </Card>
      </div>
    );
  }

  const running = server.state === "running";
  const canStart = server.state === "stopped" || server.state === "crashed";
  const alive = running || server.state === "starting" || server.state === "stopping";

  return (
    <div className="page">
      <div className="card">
        <div className="card-b row wrap" style={{ gap: 14 }}>
          <Link to="/minecraft" className="btn btn-ghost btn-sm btn-icon" title="All servers">
            <span style={{ display: "inline-flex", transform: "rotate(180deg)" }}>
              <Icon name="chevronRight" size={14} />
            </span>
          </Link>
          <div>
            <div className="row">
              <span style={{ fontSize: 17, fontWeight: 700, letterSpacing: "-0.01em" }}>
                {server.name}
              </span>
              <MCStateBadge state={server.state} />
            </div>
            <div className="small muted" style={{ marginTop: 2 }}>
              {server.software} {server.version} · port{" "}
              <span className="num">{server.port || "—"}</span>
              {server.rconEnabled && " · rcon"}
              {" · "}
              <span className="mono">{server.dir}</span>
            </div>
          </div>
          <div className="row right wrap" style={{ gap: 14 }}>
            {alive && (
              <>
                <div className="stat">
                  <span className="label">Uptime</span>
                  <span className="v" style={{ fontSize: 15 }}>
                    {fmtUptimeSince(server.startedAt)}
                  </span>
                </div>
                <div className="stat">
                  <span className="label">CPU</span>
                  <span className="v" style={{ fontSize: 15 }}>
                    {fmtPct(server.cpuPct)}
                  </span>
                </div>
                <div className="stat">
                  <span className="label">Memory</span>
                  <span className="v" style={{ fontSize: 15 }}>
                    {fmtBytes(server.memBytes)}
                    <span className="unit">/ {fmtBytes(server.memMax)}</span>
                  </span>
                </div>
                <div className="stat">
                  <span className="label">Players</span>
                  <span className="v" style={{ fontSize: 15 }}>
                    {(server.onlinePlayers ?? []).length}
                    <span className="unit">/ {server.maxPlayers ?? "—"}</span>
                  </span>
                </div>
              </>
            )}
            <div className="row" style={{ gap: 6 }}>
              {busy && <Spinner size={14} />}
              {canStart && (
                <button
                  className="btn btn-primary"
                  disabled={busy !== null || !server.eulaAccepted}
                  onClick={start}
                >
                  <Icon name="play" size={13} />
                  Start
                </button>
              )}
              {running && (
                <>
                  <button className="btn" disabled={busy !== null} onClick={restart}>
                    <Icon name="restart" size={13} />
                    Restart
                  </button>
                  <button className="btn btn-danger" disabled={busy !== null} onClick={stop}>
                    <Icon name="stop" size={13} />
                    Stop
                  </button>
                </>
              )}
              {alive && (
                <button
                  className="btn btn-danger"
                  disabled={busy !== null}
                  onClick={kill}
                  title="Force-kill (typed confirmation)"
                >
                  <Icon name="skull" size={13} />
                  Kill
                </button>
              )}
            </div>
          </div>
        </div>
        {!server.eulaAccepted && (
          <div className="card-b" style={{ paddingTop: 0 }}>
            <div className="danger-strip row">
              <Icon name="warning" size={14} />
              <span>
                Mojang's EULA has not been accepted for this server — it cannot start.
                Review it in <b>Settings</b>.
              </span>
            </div>
          </div>
        )}
        {server.lastExit && server.state === "crashed" && (
          <div className="card-b" style={{ paddingTop: 0 }}>
            <div className="danger-strip">{server.lastExit}</div>
          </div>
        )}
      </div>

      <div className="card" style={{ flex: 1 }}>
        <div className="tabs">
          {TABS.map((t) => (
            <button
              key={t}
              className={tab === t ? "tab active" : "tab"}
              onClick={() => withViewTransition(() => setTab(t))}
            >
              {t}
              {tab === t && <span className="tab-indicator" />}
            </button>
          ))}
        </div>
        <div className="tab-panel" key={tab}>
          {tab === "Console" && <ConsoleTab id={id} state={server.state} />}
          {tab === "Players" && <PlayersTab id={id} server={server} />}
          {tab === "Files" && <FilesTab id={id} />}
          {tab === "Addons" && <AddonsTab id={id} server={server} />}
          {tab === "Backups" && <BackupsTab id={id} server={server} />}
          {tab === "Tasks" && <TasksTab id={id} />}
          {tab === "Map" && <MapTab id={id} server={server} />}
          {tab === "Settings" && <SettingsTab id={id} server={server} />}
        </div>
      </div>
    </div>
  );
}
