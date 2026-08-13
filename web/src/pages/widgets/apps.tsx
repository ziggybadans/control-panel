// Application widgets: system info, temperatures, storage, services,
// Minecraft, Plex.

import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../../api/client";
import type {
  AppsResponse,
  PlexStatus,
  QbitStatus,
  QbitTorrent,
  Smart,
  StorageOverview,
} from "../../api/types";
import { fmtBytes, fmtDuration, fmtPct, fmtRate, fmtTemp } from "../../lib/format";
import { isDownloading, watchBadge } from "../../lib/qbit";
import { useFansLive, useLatestMetrics, useMCServers, useServices } from "../../state/live";
import { useSystem } from "../../state/system";
import { CapacityBar, MCStateBadge, ServiceBadge } from "../../ui/bits";
import { Icon } from "../../ui/Icon";

// --- System ----------------------------------------------------------------

export function SystemWidget() {
  const { info, uptime } = useSystem();
  const m = useLatestMetrics();
  if (!info) return null;
  return (
    <dl className="kv">
      <dt>Host</dt>
      <dd>{info.hostname}</dd>
      <dt>OS</dt>
      <dd title={info.os}>{info.os}</dd>
      <dt>Kernel</dt>
      <dd>{info.kernel}</dd>
      <dt>CPU</dt>
      <dd title={info.cpuModel}>
        {info.cpuCores}× {shortCpu(info.cpuModel)}
      </dd>
      <dt>Memory</dt>
      <dd>{fmtBytes(info.memTotal, 0)}</dd>
      <dt>Uptime</dt>
      <dd>{uptime}</dd>
      <dt>Load</dt>
      <dd>{m ? m.load.map((l) => l.toFixed(2)).join(" · ") : "—"}</dd>
    </dl>
  );
}

function shortCpu(model: string): string {
  return model
    .replace(/\((R|TM|C)\)/gi, "")
    .replace(/CPU\s*@.*$/, "")
    .replace(/\s+/g, " ")
    .trim();
}

// --- Temperatures -----------------------------------------------------------

function tempLevel(label: string, c: number): "" | "warn-text" | "crit-text" {
  const isDrive = /drivetemp|sd[a-z]/i.test(label);
  const warn = isDrive ? 45 : 75;
  const crit = isDrive ? 55 : 90;
  if (c >= crit) return "crit-text";
  if (c >= warn) return "warn-text";
  return "";
}

export function TempsWidget() {
  const m = useLatestMetrics();
  const temps = m?.temps ?? [];
  if (temps.length === 0) {
    return <div className="small faint">No temperature sensors detected.</div>;
  }
  return (
    <div className="mini-rows">
      {temps.map((t) => (
        <div key={t.label} className="mini-row">
          <span className="truncate muted small">{prettyTempLabel(t.label)}</span>
          <span className={`right num ${tempLevel(t.label, t.c)}`}>{fmtTemp(t.c)}</span>
        </div>
      ))}
    </div>
  );
}

function prettyTempLabel(label: string): string {
  return label
    .replace("coretemp Package id 0", "CPU package")
    .replace("nvme Composite", "NVMe")
    .replace("drivetemp ", "Drive ");
}

// --- Fans -------------------------------------------------------------------

export function FansWidget() {
  const { fans } = useFansLive();
  // Hide untouched 0-rpm headers (they're folded on the Fans page too).
  const list = (fans ?? []).filter(
    (f) => !(f.rpm === 0 && f.mode === "auto" && !f.err),
  );
  if (list.length === 0) {
    return <div className="small faint">No controllable fans detected.</div>;
  }
  return (
    <div className="mini-rows">
      {list.map((f) => (
        <Link
          key={f.id}
          to="/fans"
          className="mini-row"
          style={{ color: "inherit", textDecoration: "none" }}
        >
          <span className="truncate muted small" style={{ minWidth: 86 }}>
            {f.label}
          </span>
          <span style={{ flex: "1 1 40px", minWidth: 32 }}>
            <span className="bar">
              <i style={{ width: `${Math.min(f.dutyPct, 100).toFixed(0)}%` }} />
            </span>
          </span>
          <span className="right row" style={{ gap: 8, flex: "none" }}>
            {f.failsafe && <span className="dot crit" title="failsafe: sensor unreadable" />}
            {f.mode !== "auto" && (
              <span className="small faint">{f.mode}</span>
            )}
            <span className="small num muted" style={{ minWidth: 34, textAlign: "right" }}>
              {Math.round(f.dutyPct)}%
            </span>
            <span className="small num muted" style={{ minWidth: 62, textAlign: "right" }}>
              {f.rpm >= 0 ? `${f.rpm} rpm` : "—"}
            </span>
          </span>
        </Link>
      ))}
    </div>
  );
}

// --- Storage ----------------------------------------------------------------

/** Whether a mount's device is the disk itself or one of its partitions. */
function isPartOf(device: string, disk: string): boolean {
  const base = device.replace(/^\/dev\//, "");
  if (!base.startsWith(disk)) return false;
  const rest = base.slice(disk.length);
  return rest === "" || /^p?\d+$/.test(rest); // sde2, nvme0n1p2, whole disk
}

export function StorageWidget() {
  const { data } = useQuery({
    queryKey: ["storage"],
    queryFn: () => api<StorageOverview>("/api/storage"),
    refetchInterval: 30_000,
  });
  const m = useLatestMetrics();
  const pools = data?.pools ?? [];
  const disks = data?.disks ?? [];
  const mounts = data?.mounts ?? [];
  // Pool branches carry per-disk usage; index them by underlying device.
  const branchByDev = new Map(
    pools.flatMap((p) => p.branches ?? []).map((b) => [b.device, b]),
  );
  // Non-pool filesystems (root, boot, parity…) attach to their parent disk
  // as sub-rows, so system SSD partitions get usage bars too.
  const mountsFor = (disk: string) =>
    mounts.filter((mt) => isPartOf(mt.device, disk));
  const liveTemp = (name: string): number | undefined => {
    const t = m?.temps.find((t) => t.label === `drivetemp ${name}`);
    return t?.c;
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      {pools.map((p) => (
        <div key={p.mount}>
          <div className="row small" style={{ marginBottom: 5 }}>
            <span className="muted">{p.mount}</span>
            <span className="right num">
              {fmtBytes(p.used)} / {fmtBytes(p.total)}
              <span className="faint"> · {fmtPct((p.used / p.total) * 100)}</span>
            </span>
          </div>
          <CapacityBar used={p.used} total={p.total} thick />
        </div>
      ))}
      <div className="mini-rows">
        {disks.map((d) => {
          const br = branchByDev.get(d.name);
          const parts = mountsFor(d.name);
          const temp = liveTemp(d.name) ?? d.tempC;
          return (
            <div key={d.name}>
              <div className="mini-row" title={d.model}>
                <DiskHealthDot smart={d.smart} />
                <span className="mono small" style={{ minWidth: 62 }}>
                  {d.name}
                </span>
                {br ? (
                  <span style={{ flex: "1 1 56px", minWidth: 40 }}>
                    <CapacityBar used={br.used} total={br.total} />
                  </span>
                ) : (
                  <span className="small faint truncate" style={{ flex: "1 1 56px", minWidth: 0 }}>
                    {d.rotational ? "hdd" : "ssd"}
                  </span>
                )}
                <span className="right row" style={{ gap: 10, flex: "none" }}>
                  {temp !== undefined && (
                    <span
                      className={`small num ${tempLevel(d.rotational ? `drivetemp ${d.name}` : "nvme", temp)}`}
                    >
                      {fmtTemp(temp)}
                    </span>
                  )}
                  <span className="small muted num" style={{ minWidth: 52, textAlign: "right" }}>
                    {br ? fmtBytes(br.used, 1) : fmtBytes(d.sizeBytes, 0)}
                  </span>
                </span>
              </div>
              {/* Partitions with their own filesystems (root, boot, parity…). */}
              {parts.map((mt) => (
                <div
                  key={mt.mount}
                  className="mini-row"
                  style={{ paddingLeft: 22 }}
                  title={`${mt.device} · ${mt.fsType}`}
                >
                  <span className="mono small faint" style={{ minWidth: 52 }}>
                    {mt.device.replace(/^\/dev\//, "")}
                  </span>
                  <span className="small muted truncate" style={{ minWidth: 0, flex: "0 1 auto" }}>
                    {mt.mount}
                  </span>
                  <span style={{ flex: "1 1 40px", minWidth: 32 }}>
                    <CapacityBar used={mt.used} total={mt.total} />
                  </span>
                  <span
                    className="right small muted num"
                    style={{ flex: "none", minWidth: 96, textAlign: "right" }}
                  >
                    {fmtBytes(mt.used, 1)} / {fmtBytes(mt.total, 0)}
                  </span>
                </div>
              ))}
            </div>
          );
        })}
        {disks.length === 0 && <div className="small faint">No disks detected.</div>}
      </div>
      <Link to="/storage" className="small">
        Storage details →
      </Link>
    </div>
  );
}

/** SMART status as a compact dot: green ok, amber flagged, red failing. */
function DiskHealthDot({ smart }: { smart: Smart }) {
  if (smart.healthy === false) {
    return <span className="dot crit" title="SMART: failing" />;
  }
  if ((smart.reallocated ?? 0) > 0 || (smart.pendingSectors ?? 0) > 0) {
    return (
      <span
        className="dot warn"
        title={`SMART: ${smart.reallocated ?? 0} reallocated · ${smart.pendingSectors ?? 0} pending`}
      />
    );
  }
  if (!smart.available) {
    return <span className="dot off" title="SMART unavailable" />;
  }
  return <span className="dot ok" title="SMART: healthy" />;
}

// --- Services ---------------------------------------------------------------

export function ServicesWidget() {
  const services = useServices();
  const shown = services.filter((s) => s.loadState !== "not-found").slice(0, 8);
  return (
    <div className="mini-rows">
      {shown.map((s) => (
        <div key={s.unit} className="mini-row">
          <span className="truncate">{s.unit.replace(/\.(service|timer)$/, "")}</span>
          <span className="right">
            <ServiceBadge active={s.activeState} sub={s.subState} />
          </span>
        </div>
      ))}
      {shown.length === 0 && <div className="small faint">No services configured.</div>}
    </div>
  );
}

// --- Minecraft --------------------------------------------------------------

export function MinecraftWidget() {
  const servers = useMCServers();
  return (
    <div className="mini-rows">
      {servers.map((s) => (
        <Link
          key={s.id}
          to={`/minecraft/${s.id}`}
          className="mini-row"
          style={{ color: "inherit", textDecoration: "none" }}
        >
          <Icon name="minecraft" size={14} className="faint" />
          <span style={{ fontWeight: 550 }}>{s.name}</span>
          <span className="small faint">{s.version}</span>
          <span className="right row" style={{ gap: 10 }}>
            {s.state === "running" && (
              <>
                <span className="small muted num">
                  {(s.onlinePlayers ?? []).length}/{s.maxPlayers ?? "—"}{" "}
                  <Icon name="users" size={11} />
                </span>
                <span className="small muted num">{fmtPct(s.cpuPct)}</span>
                <span className="small muted num">{fmtBytes(s.memBytes, 1)}</span>
              </>
            )}
            <MCStateBadge state={s.state} />
          </span>
        </Link>
      ))}
      {servers.length === 0 && (
        <div className="small faint">No Minecraft servers discovered.</div>
      )}
    </div>
  );
}

// --- Media apps -------------------------------------------------------------

export function MediaAppsWidget() {
  const { data } = useQuery({
    queryKey: ["apps"],
    queryFn: () => api<AppsResponse>("/api/apps"),
    refetchInterval: 15_000,
  });
  const apps = data?.apps ?? [];
  if (!data?.configured) {
    return (
      <div className="small faint">
        Not configured — list your apps under <span className="mono">apps:</span> in
        config.yaml.
      </div>
    );
  }
  return (
    <div className="mini-rows">
      {apps.map((app) => {
        const issues = (app.healthIssues ?? []).length;
        return (
          <Link
            key={`${app.type}-${app.name}`}
            to="/apps"
            className="mini-row"
            style={{ color: "inherit", textDecoration: "none" }}
          >
            <span
              className={`dot ${app.reachable ? (issues > 0 ? "warn" : "ok") : "crit"}`}
            />
            <span style={{ fontWeight: 550 }}>{app.name}</span>
            <span className="right small muted num">
              {!app.reachable
                ? "unreachable"
                : app.requests
                  ? app.requests.pending > 0
                    ? `${app.requests.pending} pending`
                    : "no requests"
                  : app.queueCount > 0
                    ? `${app.queueCount} downloading`
                    : issues > 0
                      ? `${issues} issue${issues > 1 ? "s" : ""}`
                      : "idle"}
            </span>
          </Link>
        );
      })}
    </div>
  );
}

// --- Plex & downloads -------------------------------------------------------

/**
 * What is playing now, and what is on its way in: active qBittorrent
 * downloads with their rate, ETA, and whether they can be started before
 * the download finishes. Falls back to the *arr queues when qBittorrent
 * isn't configured.
 */
export function PlexWidget() {
  const { data } = useQuery({
    queryKey: ["plex"],
    queryFn: () => api<PlexStatus>("/api/plex"),
    refetchInterval: 10_000,
  });
  const { data: qbit } = useQuery({
    queryKey: ["qbit"],
    queryFn: () => api<QbitStatus>("/api/qbit"),
    refetchInterval: 6_000,
  });
  // Only needed as a stand-in when there is no download client configured.
  const { data: apps } = useQuery({
    queryKey: ["apps"],
    queryFn: () => api<AppsResponse>("/api/apps"),
    refetchInterval: 15_000,
    enabled: qbit !== undefined && !qbit.configured,
  });

  if (!data) return null;
  const sessions = data.sessions ?? [];

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      <section>
        <div className="row" style={{ marginBottom: 4 }}>
          <span className="label">Now playing</span>
          {data.configured && data.reachable && sessions.length > 0 && (
            <span className="right small num muted">
              {sessions.length} stream{sessions.length > 1 ? "s" : ""}
              {sessions.some((s) => s.decision === "transcode") &&
                ` · ${sessions.filter((s) => s.decision === "transcode").length} transcoding`}
            </span>
          )}
        </div>
        {!data.configured ? (
          <div className="small faint">
            Not configured — set <span className="mono">plex.token</span> in
            config.yaml.
          </div>
        ) : !data.reachable ? (
          <div className="small crit-text">Unreachable: {data.error}</div>
        ) : sessions.length === 0 ? (
          <div className="small faint">Nothing is playing right now.</div>
        ) : (
          <div className="mini-rows">
            {sessions.map((s, i) => (
              <div key={i} className="mini-row" style={{ alignItems: "flex-start" }}>
                <div className="grow" style={{ minWidth: 0 }}>
                  <div className="truncate" style={{ fontWeight: 550 }}>
                    {s.grandparent ? `${s.grandparent} — ${s.title}` : s.title}
                  </div>
                  <div className="small faint truncate">
                    {s.user} · {s.player}
                    {s.decision === "transcode" ? " · transcode" : ""}
                  </div>
                  <div className="bar" style={{ marginTop: 5 }}>
                    <i
                      style={{
                        width: `${s.durationMs ? ((s.progressMs / s.durationMs) * 100).toFixed(1) : 0}%`,
                      }}
                    />
                  </div>
                </div>
                {s.state === "paused" && <span className="badge neutral">paused</span>}
              </div>
            ))}
          </div>
        )}
      </section>

      <DownloadsSection qbit={qbit} apps={apps} />
    </div>
  );
}

/** Active downloads: qBittorrent when configured, else the *arr queues. */
function DownloadsSection({
  qbit,
  apps,
}: {
  qbit?: QbitStatus;
  apps?: AppsResponse;
}) {
  if (qbit && qbit.configured) {
    const active = (qbit.torrents ?? [])
      .filter((t) => isDownloading(t.state))
      .sort((a, b) => (a.watch.etaSec || a.etaSec || 1e9) - (b.watch.etaSec || b.etaSec || 1e9));
    return (
      <section>
        <div className="row" style={{ marginBottom: 4 }}>
          <span className="label">Downloading</span>
          <Link to="/qbittorrent" className="right small num muted" style={{ textDecoration: "none" }}>
            {qbit.reachable ? `${fmtRate(qbit.transfer.dlSpeed)} ↓` : ""}
          </Link>
        </div>
        {!qbit.reachable ? (
          <div className="small crit-text truncate" title={qbit.error}>
            qBittorrent unreachable: {qbit.error}
          </div>
        ) : active.length === 0 ? (
          <div className="small faint">Nothing is downloading.</div>
        ) : (
          <div className="mini-rows">
            {active.slice(0, 4).map((t) => (
              <DownloadRow key={t.hash} t={t} />
            ))}
            {active.length > 4 && (
              <Link to="/qbittorrent" className="small">
                {active.length - 4} more →
              </Link>
            )}
          </div>
        )}
      </section>
    );
  }

  // No download client configured: the *arr queues still say what is coming.
  const queue = (apps?.apps ?? []).flatMap((a) => a.queue ?? []);
  if (!apps?.configured || queue.length === 0) return null;
  return (
    <section>
      <div className="label" style={{ marginBottom: 4 }}>
        Downloading
      </div>
      <div className="mini-rows">
        {queue.slice(0, 4).map((item, i) => (
          <div key={i} className="mini-row" style={{ alignItems: "flex-start" }}>
            <div className="grow" style={{ minWidth: 0 }}>
              <div className="truncate" style={{ fontWeight: 550 }}>
                {item.title}
              </div>
              <div className="small faint truncate">
                {item.status}
                {item.timeLeft ? ` · ${item.timeLeft} left` : ""} ·{" "}
                {fmtPct(item.progress)}
              </div>
              <div className="bar" style={{ marginTop: 5 }}>
                <i style={{ width: `${Math.min(item.progress, 100).toFixed(1)}%` }} />
              </div>
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

function DownloadRow({ t }: { t: QbitTorrent }) {
  const watch = watchBadge(t.watch);
  const eta = t.watch.etaSec || t.etaSec;
  return (
    <Link
      to="/qbittorrent"
      className="mini-row"
      style={{ alignItems: "flex-start", color: "inherit", textDecoration: "none" }}
    >
      <div className="grow" style={{ minWidth: 0 }}>
        <div className="row">
          <span className="truncate" style={{ fontWeight: 550 }} title={t.name}>
            {t.media || t.name}
          </span>
          {watch.tone !== "muted" && (
            <span className={`badge ${watch.tone}`} title={watch.title}>
              {watch.label}
            </span>
          )}
          {!t.sequential && (t.watch.verdict === "now" || t.watch.verdict === "wait") && (
            <span
              className="dot warn"
              title="Sequential download is off — pieces arrive out of order, so a partial file cannot be played."
            />
          )}
        </div>
        <div className="small faint truncate">
          {t.dlSpeed > 0 ? fmtRate(t.dlSpeed) : "no rate"}
          {eta > 0 ? ` · ${fmtDuration(eta * 1000)} left` : ""} ·{" "}
          {fmtPct(t.progress * 100)} of {fmtBytes(t.sizeBytes, 0)}
        </div>
        <div className="bar" style={{ marginTop: 5 }}>
          <i style={{ width: `${Math.min(t.progress * 100, 100).toFixed(1)}%` }} />
        </div>
      </div>
    </Link>
  );
}
