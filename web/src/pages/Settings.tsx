// Settings: appearance customization, dashboard reset, safety reference,
// updates, power actions, and about.

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { UpdateStatus } from "../api/types";
import { fmtBytes } from "../lib/format";
import { usePrefs, type Accent, type Density, type Theme } from "../state/prefs";
import { useSystem } from "../state/system";
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

        <Card title="Safety model">
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }} className="small">
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
                reboot/shutdown
              </span>
            </div>
            <hr className="divider" />
            <p className="muted">
              Confirmations are enforced server-side: dangerous endpoints reject
              requests whose <span className="mono">X-Confirm</span> header does not
              echo the exact target. The agent only executes a fixed allowlist of
              operations — no free-form commands — and every action lands in the{" "}
              <a href="#/activity">audit log</a>.
            </p>
            <p className="muted">
              Backup restores keep the replaced data, snapraid runs stream their
              output, and metric collection is read-only and idles when no client is
              connected.
            </p>
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
  const { info } = useSystem();
  const toast = useToast();
  return (
    <Card title="About">
      <dl className="kv">
        <dt>Panel version</dt>
        <dd className="num">{info?.version ?? "—"}</dd>
        <dt>Mode</dt>
        <dd>{info?.mock ? "mock data" : "live"}</dd>
        <dt>Host</dt>
        <dd>{info?.hostname ?? "—"}</dd>
        <dt>OS</dt>
        <dd>{info?.os ?? "—"}</dd>
      </dl>
      <div className="row" style={{ marginTop: 12 }}>
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
    </Card>
  );
}
