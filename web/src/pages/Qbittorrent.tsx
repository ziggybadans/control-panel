// qBittorrent page: session state, the torrent list, and the small set of
// actions the panel allows (pause/resume, queue priority, sequential
// download, recheck, delete, global speed limits). Credentials stay
// server-side — the panel is not a WebUI proxy, every action goes through
// its own audited endpoint.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type {
  QbitFilesResponse,
  QbitStatus,
  QbitTorrent,
} from "../api/types";
import { fmtBytes, fmtDateTime, fmtDuration, fmtPct, fmtRate } from "../lib/format";
import { isDownloading, stateBadge, watchBadge } from "../lib/qbit";
import { externalURL } from "../lib/url";
import { TopbarActions } from "../shell/Layout";
import { Card, EmptyState, Spinner } from "../ui/bits";
import { useConfirm } from "../ui/Confirm";
import { Icon } from "../ui/Icon";
import { useToast } from "../ui/Toast";

type Filter = "all" | "downloading" | "seeding" | "stopped";

export function QbittorrentPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const [filter, setFilter] = useState<Filter>("all");
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["qbit"],
    queryFn: () => api<QbitStatus>("/api/qbit"),
    refetchInterval: 4_000,
  });

  function refresh() {
    void qc.invalidateQueries({ queryKey: ["qbit"] });
  }

  /** Runs one allowlisted action and reports the outcome. */
  async function act(
    op: string,
    body: Record<string, unknown>,
    label: string,
    confirmValue?: string,
    key = op,
  ) {
    setBusy(key);
    try {
      await api(`/api/qbit/${op}`, { method: "POST", body, confirm: confirmValue });
      toast("ok", label);
      refresh();
    } catch (e) {
      toast("error", e instanceof Error ? e.message : `${op} failed`);
    } finally {
      setBusy(null);
    }
  }

  if (isLoading || !data) {
    return (
      <div className="page">
        <div className="empty">
          <Spinner size={20} />
        </div>
      </div>
    );
  }

  if (!data.configured) {
    return (
      <div className="page">
        <Card title="qBittorrent">
          <EmptyState
            icon="torrent"
            title="qBittorrent is not configured"
            hint={
              <>
                Point <span className="mono">qbittorrent.url</span> at the WebUI in
                config.yaml, with <span className="mono">username</span> and{" "}
                <span className="mono">password</span> (omit the username when the
                instance exempts the panel's address from authentication). The
                credentials never leave the server.
              </>
            }
          />
        </Card>
      </div>
    );
  }

  const torrents = data.torrents ?? [];
  const visible = torrents.filter((t) => {
    if (search && !`${t.name} ${t.media ?? ""} ${t.category ?? ""}`
        .toLowerCase()
        .includes(search.toLowerCase())) {
      return false;
    }
    switch (filter) {
      case "downloading":
        return isDownloading(t.state);
      case "seeding":
        return t.state === "uploading" || t.state === "forcedUP" || t.state === "stalledUP";
      case "stopped":
        return t.state.startsWith("paused") || t.state.startsWith("stopped");
      default:
        return true;
    }
  });
  const active = torrents.filter((t) => isDownloading(t.state)).length;
  const tf = data.transfer;
  const canAct = data.allowActions;

  return (
    <div className="page">
      <TopbarActions>
        {canAct && (
          <button
            className={tf.altSpeed ? "btn btn-sm btn-primary" : "btn btn-sm"}
            disabled={busy !== null || !data.reachable}
            title="Alternative (throttled) speed limits"
            onClick={() =>
              void act(
                "altspeed",
                {},
                tf.altSpeed ? "speed limits back to normal" : "alternative speed limits on",
              )
            }
          >
            <Icon name="clock" size={12} />
            {tf.altSpeed ? "Alt limits on" : "Alt limits off"}
          </button>
        )}
        {data.url && (
          <a
            className="btn btn-sm"
            href={externalURL(data.url)}
            target="_blank"
            rel="noreferrer"
          >
            <Icon name="external" size={12} />
            Open WebUI
          </a>
        )}
      </TopbarActions>

      {!data.reachable ? (
        <Card title="qBittorrent">
          <div className="crit-text small">Unreachable: {data.error}</div>
          <div className="small faint" style={{ marginTop: 6 }}>
            The panel reaches the WebUI at{" "}
            <span className="mono">{data.url}</span>. When qBittorrent runs behind a
            VPN container, it is unreachable while that container is down.
          </div>
        </Card>
      ) : (
        <>
          <div className="card">
            <div className="card-b row wrap" style={{ gap: 22 }}>
              <div className="stat">
                <span className="label">Download</span>
                <span className="v" style={{ fontSize: 17 }}>
                  {fmtRate(tf.dlSpeed)}
                  {tf.dlLimit > 0 && (
                    <span className="unit">limit {fmtRate(tf.dlLimit)}</span>
                  )}
                </span>
              </div>
              <div className="stat">
                <span className="label">Upload</span>
                <span className="v" style={{ fontSize: 17 }}>
                  {fmtRate(tf.upSpeed)}
                  {tf.upLimit > 0 && (
                    <span className="unit">limit {fmtRate(tf.upLimit)}</span>
                  )}
                </span>
              </div>
              <div className="stat">
                <span className="label">Active</span>
                <span className="v" style={{ fontSize: 17 }}>
                  {active}
                  <span className="unit">/ {data.total} torrents</span>
                </span>
              </div>
              <div className="stat">
                <span className="label">Session</span>
                <span className="v" style={{ fontSize: 17 }}>
                  {fmtBytes(tf.dlData, 0)}
                  <span className="unit">down · {fmtBytes(tf.upData, 0)} up</span>
                </span>
              </div>
              {tf.freeSpace !== undefined && tf.freeSpace > 0 && (
                <div className="stat">
                  <span className="label">Free space</span>
                  <span className="v" style={{ fontSize: 17 }}>
                    {fmtBytes(tf.freeSpace, 0)}
                  </span>
                </div>
              )}
              <div className="stat">
                <span className="label">Connection</span>
                <span className="v" style={{ fontSize: 17 }}>
                  <span
                    className={`badge ${
                      tf.connection === "connected"
                        ? "ok"
                        : tf.connection === "firewalled"
                          ? "warn"
                          : "crit"
                    }`}
                  >
                    {tf.connection || "unknown"}
                  </span>
                </span>
              </div>
              <div className="stat right">
                <span className="label">Version</span>
                <span className="v num" style={{ fontSize: 17 }}>
                  {data.version || "—"}
                </span>
              </div>
            </div>
          </div>

          <div className="card">
            <div className="card-h wrap" style={{ gap: 10 }}>
              <div className="choice-row">
                {(["all", "downloading", "seeding", "stopped"] as Filter[]).map((f) => (
                  <button
                    key={f}
                    className={`choice ${filter === f ? "selected" : ""}`}
                    onClick={() => setFilter(f)}
                  >
                    {f}
                  </button>
                ))}
              </div>
              <input
                className="input"
                style={{ maxWidth: 260 }}
                placeholder="Filter by name or category…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                spellCheck={false}
              />
              <div className="row right">
                {canAct && (
                  <>
                    <button
                      className="btn btn-sm"
                      disabled={busy !== null}
                      onClick={async () => {
                        const ok = await confirm({
                          title: "Pause every torrent",
                          target: "all",
                          danger: false,
                          confirmLabel: "Pause all",
                          body: (
                            <>
                              All {data.total} torrents stop transferring until they
                              are resumed. Nothing is deleted.
                            </>
                          ),
                        });
                        if (ok) void act("pause", { hashes: ["all"] }, "all torrents paused", "all", "pause-all");
                      }}
                    >
                      Pause all
                    </button>
                    <button
                      className="btn btn-sm"
                      disabled={busy !== null}
                      onClick={async () => {
                        const ok = await confirm({
                          title: "Resume every torrent",
                          target: "all",
                          danger: false,
                          confirmLabel: "Resume all",
                          body: <>Every paused torrent starts transferring again.</>,
                        });
                        if (ok) void act("resume", { hashes: ["all"] }, "all torrents resumed", "all", "resume-all");
                      }}
                    >
                      Resume all
                    </button>
                  </>
                )}
              </div>
            </div>
            <div className="card-b flush table-scroll">
              {visible.length === 0 ? (
                <EmptyState
                  icon="torrent"
                  title={torrents.length === 0 ? "No torrents" : "Nothing matches this filter"}
                  hint={
                    torrents.length === 0
                      ? "Grabs from Radarr and Sonarr appear here as soon as qBittorrent accepts them."
                      : undefined
                  }
                />
              ) : (
                <table className="table">
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>State</th>
                      <th className="r">Size</th>
                      <th style={{ width: 120 }}>Progress</th>
                      <th className="r">Down</th>
                      <th className="r">Up</th>
                      <th className="r">Peers</th>
                      <th className="r">ETA</th>
                      <th title="Whether it can be watched before the download finishes">
                        Watchable
                      </th>
                      <th className="r" style={{ width: 96 }}></th>
                    </tr>
                  </thead>
                  <tbody>
                    {visible.map((t) => (
                      <TorrentRow
                        key={t.hash}
                        t={t}
                        canAct={canAct}
                        busy={busy === t.hash}
                        expanded={open === t.hash}
                        onToggle={() => setOpen(open === t.hash ? null : t.hash)}
                        onAct={act}
                        confirm={confirm}
                      />
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </div>

          <p className="page-note">
            <b>Watchable</b> compares how long the rest of the download takes at
            the current rate against how long the media plays for (with a 10%
            cushion), so "stream now" means playback will not catch up to the
            download. It needs a Radarr/Sonarr queue match for the runtime, and
            assumes sequential download — without it, pieces arrive out of order
            and a partial file cannot be played at all. Note that Plex only sees
            the file once the *arr import completes.
            {!canAct && (
              <>
                {" "}
                Actions are disabled (<span className="mono">
                  qbittorrent.allow_actions: false
                </span>
                ).
              </>
            )}
          </p>
        </>
      )}
    </div>
  );
}

function TorrentRow({
  t,
  canAct,
  busy,
  expanded,
  onToggle,
  onAct,
  confirm,
}: {
  t: QbitTorrent;
  canAct: boolean;
  busy: boolean;
  expanded: boolean;
  onToggle: () => void;
  onAct: (
    op: string,
    body: Record<string, unknown>,
    label: string,
    confirmValue?: string,
    key?: string,
  ) => Promise<void>;
  confirm: ReturnType<typeof useConfirm>;
}) {
  const st = stateBadge(t.state);
  const watch = watchBadge(t.watch);
  const paused = t.state.startsWith("paused") || t.state.startsWith("stopped");
  const eta = t.watch.etaSec || t.etaSec;

  async function remove() {
    const ok = await confirm({
      title: `Delete ${t.media || t.name}`,
      target: t.hash,
      confirmLabel: "Delete torrent and data",
      body: (
        <>
          <div>
            Removes the torrent <b>{t.name}</b> from qBittorrent{" "}
            <b>and deletes its downloaded data</b> from{" "}
            <span className="mono">{t.savePath || "the save path"}</span>.
          </div>
          <div className="small muted" style={{ marginTop: 8 }}>
            {t.media
              ? `${t.mediaApp ?? "The *arr app"} may re-grab this release unless you also remove it from its queue.`
              : "Media already imported into the library is not affected."}{" "}
            To keep the data on disk, use "Remove, keep data" in the row details
            instead.
          </div>
        </>
      ),
    });
    if (!ok) return;
    await onAct(
      "delete",
      { hashes: [t.hash], deleteFiles: true },
      `${t.name} deleted`,
      t.hash,
      t.hash,
    );
  }

  return (
    <>
      <tr>
        {/* The inner box bounds the column: release names are long enough to
            stretch the table past the card without it. */}
        <td>
          <div className="row" style={{ gap: 4, width: 260, maxWidth: "30vw" }}>
            <button
              className="btn btn-ghost btn-sm btn-icon"
              onClick={onToggle}
              title={expanded ? "Hide details" : "Show details"}
            >
              <Icon name={expanded ? "minus" : "plus"} size={11} />
            </button>
            <div className="grow" style={{ minWidth: 0 }}>
              <div className="truncate" style={{ fontWeight: 550 }} title={t.name}>
                {t.media || t.name}
              </div>
              {t.media ? (
                <div className="small faint truncate" title={t.name}>
                  {t.name}
                </div>
              ) : (
                t.category && <div className="small faint truncate">{t.category}</div>
              )}
            </div>
          </div>
        </td>
        <td>
          <span className={`badge ${st.tone}`}>{st.label}</span>
        </td>
        <td className="r num small">{fmtBytes(t.sizeBytes, 1)}</td>
        <td>
          <div className="row" style={{ gap: 8, minWidth: 100 }}>
            <span className="bar grow">
              <i style={{ width: `${Math.min(t.progress * 100, 100).toFixed(1)}%` }} />
            </span>
            <span className="small num muted" style={{ minWidth: 34, textAlign: "right" }}>
              {fmtPct(t.progress * 100)}
            </span>
          </div>
        </td>
        <td className="r num small">{t.dlSpeed > 0 ? fmtRate(t.dlSpeed) : "—"}</td>
        <td className="r num small">{t.upSpeed > 0 ? fmtRate(t.upSpeed) : "—"}</td>
        <td className="r num small" title="connected seeds / connected peers">
          {t.seeds}/{t.peers}
        </td>
        <td className="r num small">{eta > 0 ? fmtDuration(eta * 1000) : "—"}</td>
        <td>
          <span className="row" style={{ gap: 5 }}>
            {watch.tone === "muted" ? (
              <span className="small faint" title={watch.title}>
                {watch.label}
              </span>
            ) : (
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
          </span>
        </td>
        <td className="r">
          {canAct && (
            <div className="row" style={{ justifyContent: "flex-end", gap: 2 }}>
              <button
                className="btn btn-ghost btn-sm btn-icon"
                disabled={busy}
                title={paused ? "Resume" : "Pause"}
                onClick={() =>
                  void onAct(
                    paused ? "resume" : "pause",
                    { hashes: [t.hash] },
                    `${t.name} ${paused ? "resumed" : "paused"}`,
                    undefined,
                    t.hash,
                  )
                }
              >
                <Icon name={paused ? "play" : "stop"} size={11} />
              </button>
              <button
                className="btn btn-ghost btn-sm btn-icon crit-text"
                disabled={busy}
                title="Delete torrent and data"
                onClick={() => void remove()}
              >
                <Icon name="trash" size={12} />
              </button>
            </div>
          )}
        </td>
      </tr>
      {expanded && (
        <tr>
          <td colSpan={10} style={{ background: "var(--surface-2)" }}>
            <TorrentDetail
              t={t}
              canAct={canAct}
              busy={busy}
              onAct={onAct}
              confirm={confirm}
            />
          </td>
        </tr>
      )}
    </>
  );
}

function TorrentDetail({
  t,
  canAct,
  busy,
  onAct,
  confirm,
}: {
  t: QbitTorrent;
  canAct: boolean;
  busy: boolean;
  onAct: (
    op: string,
    body: Record<string, unknown>,
    label: string,
    confirmValue?: string,
    key?: string,
  ) => Promise<void>;
  confirm: ReturnType<typeof useConfirm>;
}) {
  const { data, isLoading } = useQuery({
    queryKey: ["qbit-files", t.hash],
    queryFn: () => api<QbitFilesResponse>(`/api/qbit/torrents/${t.hash}/files`),
    staleTime: 10_000,
  });
  const files = data?.files ?? [];

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12, padding: "4px 0 8px" }}>
      <div className="row wrap" style={{ gap: 24, alignItems: "flex-start" }}>
        <dl className="kv" style={{ flex: "1 1 300px", minWidth: 0 }}>
          <dt>Saved to</dt>
          <dd className="mono small">{t.savePath || "—"}</dd>
          <dt>Added</dt>
          <dd className="num">{t.addedOn ? fmtDateTime(t.addedOn * 1000) : "—"}</dd>
          {t.completedOn ? (
            <>
              <dt>Completed</dt>
              <dd className="num">{fmtDateTime(t.completedOn * 1000)}</dd>
            </>
          ) : null}
          <dt>Downloaded</dt>
          <dd className="num">
            {fmtBytes(t.downloaded)} of {fmtBytes(t.sizeBytes)}
          </dd>
          <dt>Ratio</dt>
          <dd className="num">{t.ratio.toFixed(2)}</dd>
          <dt>Swarm</dt>
          <dd className="num">
            {t.seeds}/{t.seedsTotal} seeds · {t.peers}/{t.peersTotal} peers
            {t.availability ? ` · ${t.availability.toFixed(2)} availability` : ""}
          </dd>
          {t.media && (
            <>
              <dt>Matched</dt>
              <dd>
                {t.media}
                <span className="faint">
                  {" "}
                  · {t.mediaApp}
                  {t.runtimeSec ? ` · ${fmtDuration(t.runtimeSec * 1000)} runtime` : ""}
                </span>
              </dd>
            </>
          )}
          <dt>Queue position</dt>
          <dd className="num">{t.priority > 0 ? `#${t.priority}` : "not queued"}</dd>
        </dl>

        {canAct && (
          <div
            style={{
              display: "flex",
              flexDirection: "column",
              gap: 8,
              flex: "1 1 260px",
              minWidth: 0,
            }}
          >
            <span className="label">Actions</span>
            <div className="row wrap" style={{ gap: 6 }}>
              <button
                className={t.sequential ? "btn btn-sm btn-primary" : "btn btn-sm"}
                disabled={busy}
                title="Download pieces in order — required to play a file before it finishes"
                onClick={() =>
                  void onAct(
                    "sequential",
                    { hashes: [t.hash] },
                    `sequential download ${t.sequential ? "off" : "on"}`,
                    undefined,
                    t.hash,
                  )
                }
              >
                Sequential {t.sequential ? "on" : "off"}
              </button>
              <button
                className={t.firstLast ? "btn btn-sm btn-primary" : "btn btn-sm"}
                disabled={busy}
                title="Prioritise the first and last pieces so players can read the container header"
                onClick={() =>
                  void onAct(
                    "firstlast",
                    { hashes: [t.hash] },
                    `first/last piece priority ${t.firstLast ? "off" : "on"}`,
                    undefined,
                    t.hash,
                  )
                }
              >
                First/last {t.firstLast ? "on" : "off"}
              </button>
              <button
                className="btn btn-sm"
                disabled={busy}
                title="Move to the top of the download queue"
                onClick={() =>
                  void onAct("top", { hashes: [t.hash] }, "moved to the top of the queue", undefined, t.hash)
                }
              >
                Top of queue
              </button>
              <button
                className="btn btn-sm"
                disabled={busy}
                title="Re-verify the downloaded data against the torrent"
                onClick={() =>
                  void onAct("recheck", { hashes: [t.hash] }, "recheck started", undefined, t.hash)
                }
              >
                <Icon name="refresh" size={12} />
                Recheck
              </button>
              <button
                className="btn btn-sm btn-danger"
                disabled={busy}
                title="Stop managing this torrent but leave the downloaded files on disk"
                onClick={async () => {
                  const ok = await confirm({
                    title: `Remove ${t.media || t.name}`,
                    target: t.hash,
                    confirmLabel: "Remove, keep data",
                    body: (
                      <>
                        Removes <b>{t.name}</b> from qBittorrent (seeding stops).
                        The downloaded files stay in{" "}
                        <span className="mono">{t.savePath || "the save path"}</span>.
                      </>
                    ),
                  });
                  if (ok) {
                    void onAct(
                      "delete",
                      { hashes: [t.hash], deleteFiles: false },
                      `${t.name} removed`,
                      t.hash,
                      t.hash,
                    );
                  }
                }}
              >
                Remove, keep data
              </button>
            </div>
          </div>
        )}
      </div>

      <div>
        <span className="label">Files</span>
        {isLoading ? (
          <div className="row small muted" style={{ marginTop: 6 }}>
            <Spinner size={12} /> loading…
          </div>
        ) : files.length === 0 ? (
          <div className="small faint" style={{ marginTop: 6 }}>
            No file list available.
          </div>
        ) : (
          <div className="mini-rows" style={{ marginTop: 4 }}>
            {files.slice(0, 12).map((f) => (
              <div key={f.name} className="mini-row">
                <Icon name="file" size={12} className="faint" />
                <span className="truncate small" title={f.name}>
                  {f.name}
                </span>
                <span className="right row" style={{ gap: 10, flex: "none" }}>
                  {f.priority === 0 && <span className="badge neutral">skipped</span>}
                  <span className="small num muted" style={{ minWidth: 58, textAlign: "right" }}>
                    {fmtBytes(f.sizeBytes, 1)}
                  </span>
                  <span className="small num muted" style={{ minWidth: 40, textAlign: "right" }}>
                    {fmtPct(f.progress * 100)}
                  </span>
                </span>
              </div>
            ))}
            {files.length > 12 && (
              <div className="small faint">+{files.length - 12} more files</div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
