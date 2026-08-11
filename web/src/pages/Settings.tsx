// Settings: appearance customization, dashboard reset, safety reference,
// power actions, and about.

import { useState } from "react";
import { api } from "../api/client";
import { usePrefs, type Accent, type Density, type Theme } from "../state/prefs";
import { useSystem } from "../state/system";
import { Card } from "../ui/bits";
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

        <PowerCard />
        <AboutCard />
      </div>
    </div>
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
