// Settings: appearance customization, dashboard reset, safety reference,
// updates, power actions, and about.

import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { UpdateStatus } from "../api/types";
import { fmtBytes, fmtDuration } from "../lib/format";
import { usePrefs, type Accent, type Density, type Theme } from "../state/prefs";
import { usePanel, useSystem } from "../state/system";
import { Card, Spinner } from "../ui/bits";
import { useConfirm } from "../ui/Confirm";
import { Icon } from "../ui/Icon";
import { useToast } from "../ui/Toast";

const ACCENTS: { id: Accent; color: string }[] = [
  { id: "blue", color: "#4f96f0" },
  { id: "teal", color: "#2fbdb3" },
  { id: "violet", color: "#9d87f2" },
  { id: "green", color: "#4cb85c" },
  { id: "amber", color: "#d9a62e" },
  { id: "rose", color: "#e56b8c" },
];

export function SettingsPage() {
  const { prefs, update, resetDashboard } = usePrefs();
  const toast = useToast();

  return (
    <div className="page">
      <div className="settings-grid">
        <Card title="Appearance">
          <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            <div className="field">
              <span className="label">Theme</span>
              <div className="choice-row">
                {(["dark", "light", "system"] as Theme[]).map((t) => (
                  <button
                    key={t}
                    className={`choice ${prefs.theme === t ? "selected" : ""}`}
                    onClick={() => update({ theme: t })}
                  >
                    {t}
                  </button>
                ))}
              </div>
            </div>
            <div className="field">
              <span className="label">Accent</span>
              <div className="swatches">
                {ACCENTS.map((a) => (
                  <button
                    key={a.id}
                    className={`swatch ${prefs.accent === a.id ? "selected" : ""}`}
                    style={{ background: a.color }}
                    title={a.id}
                    onClick={() => update({ accent: a.id })}
                  />
                ))}
              </div>
              <span className="hint">Charts keep their fixed, accessibility-validated palette.</span>
            </div>
            <div className="field">
              <span className="label">Density</span>
              <div className="choice-row">
                {(["compact", "comfortable"] as Density[]).map((d) => (
                  <button
                    key={d}
                    className={`choice ${prefs.density === d ? "selected" : ""}`}
                    onClick={() => update({ density: d })}
                  >
                    {d}
                  </button>
                ))}
              </div>
            </div>
            <div className="setting-row" style={{ borderBottom: "none", paddingBottom: 0 }}>
              <div className="desc">
                <div className="t">Dashboard layout</div>
                <div className="s">Restore the default widget arrangement</div>
              </div>
              <button
                className="btn btn-sm"
                onClick={() => {
                  resetDashboard();
                  toast("ok", "dashboard layout reset");
                }}
              >
                Reset
              </button>
            </div>
            <div className="small faint">
              Preferences are stored on the server, so they follow you to any browser.
            </div>
          </div>
        </Card>

        <UpdatesCard />
        <PowerCard />
        <AboutCard />
      </div>
    </div>
  );
}

/** Strips the leading "v" so tags compare against build versions. */
function vNormalize(v: string): string {
  return v.replace(/^v/, "");
}

function UpdatesCard() {
  const confirm = useConfirm();
  const toast = useToast();
  const qc = useQueryClient();
  const { data } = useQuery({
    queryKey: ["update"],
    queryFn: () => api<UpdateStatus>("/api/update"),
    staleTime: 60_000,
  });
  const [checking, setChecking] = useState(false);
  const [phase, setPhase] = useState<"idle" | "installing" | "restarting">("idle");

  async function check() {
    setChecking(true);
    try {
      const st = await api<UpdateStatus>("/api/update?refresh=1");
      qc.setQueryData(["update"], st);
      if (st.error) toast("error", st.error);
      else if (st.updateAvailable) toast("ok", `update available: ${st.latest?.tag}`);
      else toast("ok", "panel is up to date");
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "update check failed");
    } finally {
      setChecking(false);
    }
  }

  async function install() {
    const tag = data?.latest?.tag;
    if (!tag) return;
    const ok = await confirm({
      title: "Install panel update",
      target: tag,
      typed: true,
      body: (
        <>
          Downloads <b>{tag}</b> from <span className="mono">{data?.repo}</span>,
          verifies its checksum, replaces the panel binary, and restarts the
          panel service. Running Minecraft servers are stopped gracefully and
          resumed after the restart; expect the panel to be away for ~10 seconds.
        </>
      ),
      confirmLabel: "Install & restart",
    });
    if (!ok) return;
    setPhase("installing");
    try {
      await api("/api/update/apply", { method: "POST", body: { tag }, confirm: tag });
      setPhase("restarting");
      // Wait for the panel to go down and come back on the new version.
      // If it never goes down (mock mode), settle after a few steady polls.
      const deadline = Date.now() + 120_000;
      let sawDown = false;
      let steady = 0;
      await new Promise((r) => setTimeout(r, 3000));
      while (Date.now() < deadline) {
        try {
          const h = await api<{ version: string }>("/api/health");
          if (vNormalize(h.version) === vNormalize(tag) || sawDown || ++steady >= 5) break;
        } catch {
          sawDown = true;
        }
        await new Promise((r) => setTimeout(r, 2000));
      }
      window.location.reload();
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "update failed");
      setPhase("idle");
    }
  }

  const latest = data?.latest;
  return (
    <Card title="Updates">
      <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
        <div className="row small">
          <span className="muted">Installed</span>
          <span className="num">{data?.current ?? "—"}</span>
          {data?.configured && latest && (
            <>
              <span className="faint">·</span>
              <span className="muted">Latest</span>
              <span className="num">{latest.tag}</span>
            </>
          )}
          {data?.configured &&
            (data.updateAvailable ? (
              <span className="badge warn right">update available</span>
            ) : data.error ? (
              <span className="badge crit right">check failed</span>
            ) : (
              <span className="badge ok right">up to date</span>
            ))}
        </div>

        {!data?.configured && (
          <div className="small faint">
            Not configured — set <span className="mono">update.repo</span> in
            config.yaml to enable in-panel updates from GitHub releases.
          </div>
        )}
        {data?.error && <div className="small crit-text">{data.error}</div>}

        {data?.updateAvailable && latest && (
          <div className="small">
            <div className="row faint" style={{ marginBottom: 4 }}>
              <span className="num">
                {latest.publishedAt ? latest.publishedAt.slice(0, 10) : ""}
              </span>
              <span>· {fmtBytes(latest.assetSize)}</span>
            </div>
            {latest.notes && (
              <div
                className="mono muted"
                style={{ whiteSpace: "pre-wrap", maxHeight: 140, overflowY: "auto" }}
              >
                {latest.notes}
              </div>
            )}
          </div>
        )}

        <div className="row">
          <button className="btn btn-sm" disabled={checking || phase !== "idle"} onClick={check}>
            {checking ? <Spinner size={11} /> : <Icon name="restart" size={12} />}
            Check for updates
          </button>
          {data?.updateAvailable && latest && (
            <button
              className="btn btn-sm btn-danger"
              disabled={phase !== "idle"}
              onClick={install}
            >
              {phase !== "idle" ? <Spinner size={11} /> : <Icon name="download" size={12} />}
              {phase === "installing"
                ? "Installing…"
                : phase === "restarting"
                  ? "Restarting…"
                  : `Install ${latest.tag}`}
            </button>
          )}
        </div>
        <div className="small faint">
          Updates install only from the pinned repo, are sha256-verified against
          the release manifest, and keep the previous binary as{" "}
          <span className="mono">control-panel.old</span> for rollback.
        </div>
      </div>
    </Card>
  );
}

function PowerCard() {
  const confirm = useConfirm();
  const toast = useToast();
  const { info } = useSystem();
  const [busy, setBusy] = useState(false);

  async function power(action: "reboot" | "shutdown") {
    const ok = await confirm({
      title: action === "reboot" ? "Reboot server" : "Shut down server",
      target: action,
      typed: true,
      body:
        action === "reboot" ? (
          <>
            <b>{info?.hostname}</b> will reboot. Storage, Plex, and all Minecraft
            servers go down until it is back. The panel stops Minecraft servers
            gracefully first.
          </>
        ) : (
          <>
            <b>{info?.hostname}</b> will power off and must be started physically (or
            via Wake-on-LAN). Everything it serves goes offline.
          </>
        ),
      confirmLabel: action === "reboot" ? "Reboot" : "Shut down",
    });
    if (!ok) return;
    setBusy(true);
    try {
      await api(`/api/power/${action}`, { method: "POST", confirm: action });
      toast("ok", `${action} initiated`);
    } catch (e) {
      toast("error", e instanceof Error ? e.message : `${action} failed`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card title="Power">
      <div className="setting-row" style={{ borderBottom: "none", padding: 0 }}>
        <div className="desc">
          <div className="t">Host power control</div>
          <div className="s">
            Requires <span className="mono">power.allow: true</span> in config.yaml.
            Both actions use typed confirmation.
          </div>
        </div>
        <div className="row">
          <button className="btn btn-sm" disabled={busy} onClick={() => power("reboot")}>
            <Icon name="restart" size={12} />
            Reboot
          </button>
          <button className="btn btn-sm btn-danger" disabled={busy} onClick={() => power("shutdown")}>
            <Icon name="power" size={12} />
            Shut down
          </button>
        </div>
      </div>
    </Card>
  );
}

function AboutCard() {
  const { info, uptime } = useSystem();
  const panel = usePanel();
  const toast = useToast();
  const [showSafety, setShowSafety] = useState(false);
  const f = panel?.features;

  const integrations: { label: string; on: boolean }[] = f
    ? [
        { label: "SMART", on: f.smart },
        { label: "snapraid", on: f.snapraid },
        { label: "Plex", on: f.plex },
        { label: `${f.apps} media app${f.apps === 1 ? "" : "s"}`, on: f.apps > 0 },
        { label: "power control", on: f.power },
        { label: "self-update", on: !!f.updateRepo },
        { label: "fan control", on: f.fanControl },
        {
          label: f.fileRoots > 0 ? `${f.fileRoots} file root${f.fileRoots === 1 ? "" : "s"}` : "file manager",
          on: f.fileRoots > 0,
        },
        { label: "terminal", on: f.terminal },
      ]
    : [];

  return (
    <Card title="About">
      <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
        <div>
          <div className="label" style={{ marginBottom: 6 }}>
            Panel
          </div>
          <dl className="kv">
            <dt>Version</dt>
            <dd className="num">
              {info?.version ?? "—"}
              {info?.mock && <span className="badge neutral" style={{ marginLeft: 8 }}>mock data</span>}
            </dd>
            <dt>Runtime</dt>
            <dd className="num">{panel ? `${panel.goVersion} · pid ${panel.pid}` : "—"}</dd>
            <dt>Panel uptime</dt>
            <dd className="num">{panel ? fmtDuration(Date.now() - panel.startedAt) : "—"}</dd>
            <dt>Listen</dt>
            <dd className="num">
              {panel?.listen ?? "—"}
              {panel && (
                <span className={panel.tls ? "badge ok" : "badge neutral"} style={{ marginLeft: 8 }}>
                  {panel.tls ? "TLS" : "no TLS"}
                </span>
              )}
            </dd>
            <dt>Auth</dt>
            <dd>
              {panel
                ? panel.authMode === "none"
                  ? "disabled"
                  : `password · ${Math.round(panel.sessionHours / 24)}d sessions`
                : "—"}
            </dd>
            <dt>Data dir</dt>
            <dd className="mono small">{panel?.dataDir ?? "—"}</dd>
            {f?.updateRepo && (
              <>
                <dt>Update repo</dt>
                <dd className="mono small">{f.updateRepo}</dd>
              </>
            )}
          </dl>
        </div>

        <div>
          <div className="label" style={{ marginBottom: 6 }}>
            Host
          </div>
          <dl className="kv">
            <dt>Hostname</dt>
            <dd>{info?.hostname ?? "—"}</dd>
            <dt>OS</dt>
            <dd title={info?.os}>{info?.os ?? "—"}</dd>
            <dt>Kernel</dt>
            <dd className="num">
              {info ? `${info.kernel} · ${info.arch}` : "—"}
            </dd>
            <dt>Uptime</dt>
            <dd className="num">{uptime}</dd>
          </dl>
        </div>

        {integrations.length > 0 && (
          <div>
            <div className="label" style={{ marginBottom: 6 }}>
              Integrations
            </div>
            <div className="row wrap" style={{ gap: 6 }}>
              {integrations.map((i) => (
                <span key={i.label} className={i.on ? "badge ok" : "badge neutral"}>
                  {i.label}
                </span>
              ))}
            </div>
          </div>
        )}

        <div className="row">
          <button className="btn btn-sm" onClick={() => setShowSafety(true)}>
            <Icon name="lock" size={12} />
            Safety model
          </button>
          <a
            className="btn btn-sm"
            href="https://github.com/ziggybadans/control-panel"
            target="_blank"
            rel="noreferrer"
          >
            <Icon name="external" size={12} />
            GitHub
          </a>
          <button
            className="btn btn-sm"
            onClick={async () => {
              try {
                await api("/api/auth/logout", { method: "POST" });
              } finally {
                toast("ok", "signed out");
                window.location.reload();
              }
            }}
          >
            <Icon name="logout" size={12} />
            Sign out
          </button>
        </div>
      </div>
      {showSafety && <SafetyModal onClose={() => setShowSafety(false)} />}
    </Card>
  );
}

/** The full safety reference, opened from the About card. */
function SafetyModal({ onClose }: { onClose: () => void }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div
      className="modal-overlay"
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
    >
      <div className="modal wide" role="dialog" aria-modal="true" aria-label="Safety model">
        <div className="modal-h">
          <Icon name="lock" size={16} />
          Safety model
        </div>
        <div
          className="modal-b small"
          style={{ maxHeight: "62vh", overflowY: "auto", display: "flex", flexDirection: "column", gap: 12 }}
        >
          <div>
            <div className="label" style={{ marginBottom: 6 }}>
              Confirmation tiers
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <div className="row">
                <span className="badge neutral">confirm</span>
                <span className="muted">
                  service stop/restart · Minecraft stop/restart · kick
                </span>
              </div>
              <div className="row">
                <span className="badge crit">typed confirm</span>
                <span className="muted">
                  force-kill · backup restore/delete · ban · snapraid sync/scrub ·
                  folder delete · terminal session · reboot/shutdown · panel
                  updates
                </span>
              </div>
            </div>
            <p className="muted" style={{ marginTop: 8 }}>
              Confirmations are enforced <em>server-side</em>: dangerous endpoints
              reject any request whose <span className="mono">X-Confirm</span> header
              does not echo the exact target (HTTP 428). The dialogs in this UI are
              just the way that header gets filled in — a buggy or malicious page
              cannot skip the check.
            </p>
          </div>

          <div>
            <div className="label" style={{ marginBottom: 6 }}>
              Command execution
            </div>
            <p className="muted">
              The panel only executes a fixed allowlist of operations, built as
              argv arrays — never shell strings. systemd verbs run only against
              the units listed in config, snapraid runs only its four known
              subcommands, and Minecraft servers are supervised child processes
              (optionally de-privileged to <span className="mono">run_as</span>).
            </p>
            <p className="muted">
              The <b>Terminal</b> page is the one deliberate exception: it is a
              real shell. It stays off unless{" "}
              <span className="mono">terminal.enabled</span> is set, runs as the
              unprivileged <span className="mono">terminal.run_as</span> user
              (mandatory when the panel runs as root, unless root shells are
              explicitly accepted), needs typed confirmation to open, caps
              concurrent sessions, closes idle ones, and logs every session
              open/close to the audit trail.
            </p>
          </div>

          <div>
            <div className="label" style={{ marginBottom: 6 }}>
              File confinement
            </div>
            <p className="muted">
              Every file operation resolves paths inside its configured root —
              lexically (no <span className="mono">..</span>) and physically
              (symlinks may not lead outside). Uploads refuse to overwrite unless
              asked, archives are extracted with zip-bomb and path-escape guards,
              and backup restores keep the replaced data.
            </p>
          </div>

          <div>
            <div className="label" style={{ marginBottom: 6 }}>
              Authentication &amp; transport
            </div>
            <p className="muted">
              Login uses argon2id password hashing with rate limiting (5 tries
              per 15 minutes per address). Sessions are random 256-bit tokens in
              an HttpOnly cookie. Mutations require the{" "}
              <span className="mono">X-CP</span> header and a same-origin{" "}
              <span className="mono">Origin</span>, which cross-site forms cannot
              produce. A strict CSP and frame-ancestors deny embedding. Updates
              install only from the pinned GitHub repo and are sha256-verified
              against the release manifest.
            </p>
          </div>

          <div>
            <div className="label" style={{ marginBottom: 6 }}>
              Audit
            </div>
            <p className="muted">
              Every mutating action — who, what, when, from where, and whether it
              succeeded — is appended to a JSONL audit log in the data dir and
              visible on the <a href="#/activity">Activity</a> page. Metric
              collection is read-only and idles when no client is connected.
            </p>
          </div>
        </div>
        <div className="modal-f">
          <button className="btn btn-primary" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  );
}
