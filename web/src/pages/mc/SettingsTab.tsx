// Server settings: EULA, launch configuration (panel-managed overrides),
// and the server.properties editor (comment-preserving on the backend).

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { api } from "../../api/client";
import type { FilesResponse, MCServer, PropEntry } from "../../api/types";
import { useJobs } from "../../state/live";
import { Spinner, Toggle } from "../../ui/bits";
import { useConfirm } from "../../ui/Confirm";
import { Icon } from "../../ui/Icon";
import { useToast } from "../../ui/Toast";
import { JobPanel } from "../Storage";

export function SettingsTab({ id, server }: { id: string; server: MCServer }) {
  return (
    <div className="card-b" style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      {!server.eulaAccepted && <EulaSection id={id} />}
      <LaunchSection id={id} server={server} />
      <JarSection id={id} server={server} />
      <PropertiesSection id={id} serverState={server.state} />
    </div>
  );
}

const JAR_FLAVORS = ["paper", "purpur", "fabric", "vanilla"];

function JarSection({ id, server }: { id: string; server: MCServer }) {
  const toast = useToast();
  const confirm = useConfirm();
  const jobs = useJobs();
  const [flavor, setFlavor] = useState(
    JAR_FLAVORS.includes((server.software ?? "").toLowerCase())
      ? (server.software ?? "").toLowerCase()
      : "paper",
  );
  const [version, setVersion] = useState("");

  const { data: versions } = useQuery({
    queryKey: ["mc-versions", flavor],
    queryFn: () =>
      api<{ versions: string[] }>(`/api/minecraft/meta/versions?flavor=${flavor}`),
    staleTime: 3600_000,
  });
  const selected = version || versions?.versions?.[0] || "";
  const jarJob = jobs.find((j) => j.kind === "mc.jar" && j.target === id);
  const [showJob, setShowJob] = useState(false);

  async function update() {
    const ok = await confirm({
      title: `Update ${id} to ${flavor} ${selected}`,
      target: id,
      body: (
        <>
          The panel downloads the official {flavor} {selected} server jar into the
          server folder and switches the launch configuration to it. The current jar
          stays on disk for rollback. Back up first if this is a major version jump —
          world upgrades are one-way.
        </>
      ),
      confirmLabel: "Download & switch",
    });
    if (!ok) return;
    try {
      await api(`/api/minecraft/${id}/jar`, {
        method: "POST",
        body: { flavor, version: selected },
        confirm: id,
      });
      setShowJob(true);
      toast("ok", "jar update started");
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "update failed");
    }
  }

  return (
    <section>
      <div className="row" style={{ marginBottom: 8 }}>
        <span className="label">Server jar</span>
        <span className="small faint mono">{server.jar || "run.sh"}</span>
      </div>
      <div className="row wrap" style={{ gap: 8 }}>
        <div className="choice-row">
          {JAR_FLAVORS.map((f) => (
            <button
              key={f}
              className={`choice ${flavor === f ? "selected" : ""}`}
              onClick={() => {
                setFlavor(f);
                setVersion("");
              }}
            >
              {f}
            </button>
          ))}
        </div>
        <select
          className="select mono"
          value={selected}
          onChange={(e) => setVersion(e.target.value)}
        >
          {(versions?.versions ?? []).slice(0, 30).map((v) => (
            <option key={v} value={v}>
              {v}
            </option>
          ))}
        </select>
        <button
          className="btn btn-sm btn-primary"
          disabled={!selected || jarJob?.state === "running"}
          onClick={update}
        >
          <Icon name="download" size={12} />
          Update jar
        </button>
        <span className="small faint">applies on next start · a jar can also be uploaded in Files</span>
      </div>
      <ExistingJarRow id={id} server={server} />
      {jarJob && (
        <div style={{ marginTop: 8 }}>
          <JobPanel
            job={jarJob}
            expanded={showJob || jarJob.state === "running"}
            onToggle={() => setShowJob(!showJob)}
          />
        </div>
      )}
    </section>
  );
}

function EulaSection({ id }: { id: string }) {
  const confirm = useConfirm();
  const toast = useToast();

  async function accept() {
    const ok = await confirm({
      title: "Accept the Minecraft EULA",
      target: id,
      danger: false,
      body: (
        <>
          Starting a server requires agreeing to the{" "}
          <a href="https://aka.ms/MinecraftEULA" target="_blank" rel="noreferrer">
            Minecraft End User License Agreement
          </a>
          . The panel will write <span className="mono">eula=true</span> for{" "}
          <b>{id}</b> on your behalf.
        </>
      ),
      confirmLabel: "Accept EULA",
    });
    if (!ok) return;
    try {
      await api(`/api/minecraft/${id}/eula`, { method: "POST", confirm: id });
      toast("ok", "EULA accepted");
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "failed");
    }
  }

  return (
    <section className="danger-strip row">
      <Icon name="warning" size={14} />
      <span className="grow">
        The Mojang EULA has not been accepted; the server cannot start.
      </span>
      <button className="btn btn-sm btn-primary" onClick={accept}>
        Review &amp; accept
      </button>
    </section>
  );
}

/**
 * Point the launch configuration at a jar that's already in the server
 * directory — covers imported servers and modpack launchers the download
 * flavors don't offer.
 */
function ExistingJarRow({ id, server }: { id: string; server: MCServer }) {
  const toast = useToast();
  const confirm = useConfirm();
  const [choice, setChoice] = useState("");

  const { data: rootFiles } = useQuery({
    queryKey: ["mc-files", id, ""],
    queryFn: () => api<FilesResponse>(`/api/minecraft/${id}/files?path=`),
    staleTime: 30_000,
  });
  const jars = (rootFiles?.entries ?? [])
    .filter((e) => !e.dir && e.name.endsWith(".jar"))
    .map((e) => e.name);

  if (jars.length === 0) return null;
  const selected = choice || jars.find((j) => j !== server.jar) || jars[0];

  async function useJar() {
    const ok = await confirm({
      title: `Switch ${id} to ${selected}`,
      target: id,
      body: (
        <>
          The launch configuration will point at{" "}
          <span className="mono">{selected}</span> (already in the server
          folder) instead of <span className="mono">{server.jar || "auto-detection"}</span>.
          Nothing is downloaded or deleted; applies on next start.
        </>
      ),
      confirmLabel: "Use this jar",
    });
    if (!ok) return;
    try {
      await api(`/api/minecraft/${id}/config`, {
        method: "PUT",
        body: { jar: selected },
      });
      toast("ok", `now using ${selected} (applies on next start)`);
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "switch failed");
    }
  }

  return (
    <div className="row wrap" style={{ gap: 8, marginTop: 8 }}>
      <span className="small muted">or use an existing jar:</span>
      <select
        className="select mono"
        value={selected}
        onChange={(e) => setChoice(e.target.value)}
      >
        {jars.map((j) => (
          <option key={j} value={j}>
            {j}
            {j === server.jar ? " (current)" : ""}
          </option>
        ))}
      </select>
      <button
        className="btn btn-sm"
        disabled={selected === server.jar}
        onClick={useJar}
      >
        Use this jar
      </button>
    </div>
  );
}

function LaunchSection({ id, server }: { id: string; server: MCServer }) {
  const toast = useToast();
  const [mem, setMem] = useState(server.mem ?? "");
  const [jvmArgs, setJvmArgs] = useState((server.jvmArgs ?? []).join(" "));
  const [aikar, setAikar] = useState(server.aikar);
  const [autoStart, setAutoStart] = useState(server.autoStart);
  const [autoRestart, setAutoRestart] = useState(server.autoRestart);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setMem(server.mem ?? "");
    setJvmArgs((server.jvmArgs ?? []).join(" "));
    setAikar(server.aikar);
    setAutoStart(server.autoStart);
    setAutoRestart(server.autoRestart);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  const dirty =
    mem !== (server.mem ?? "") ||
    jvmArgs !== (server.jvmArgs ?? []).join(" ") ||
    aikar !== server.aikar ||
    autoStart !== server.autoStart ||
    autoRestart !== server.autoRestart;

  async function save() {
    setSaving(true);
    try {
      await api(`/api/minecraft/${id}/config`, {
        method: "PUT",
        body: { mem, jvmArgs, aikar, autoStart, autoRestart },
      });
      toast("ok", "launch settings saved (applies on next start)");
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <section>
      <div className="row" style={{ marginBottom: 8 }}>
        <span className="label">Launch configuration</span>
        <span className="small faint">applies on next start</span>
        {dirty && (
          <button className="btn btn-sm btn-primary right" onClick={save} disabled={saving}>
            {saving ? <Spinner size={12} /> : <Icon name="check" size={12} />}
            Save
          </button>
        )}
      </div>
      <div className="grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))" }}>
        <div className="field">
          <span className="label">Memory (Xms/Xmx)</span>
          <input
            className="input mono"
            value={mem}
            placeholder="e.g. 6G"
            onChange={(e) => setMem(e.target.value)}
            spellCheck={false}
          />
          <span className="hint">Java heap size, e.g. 4G or 6144M</span>
        </div>
        <div className="field">
          <span className="label">Extra JVM arguments</span>
          <input
            className="input mono"
            value={jvmArgs}
            placeholder="-Dfml.readTimeout=180 …"
            onChange={(e) => setJvmArgs(e.target.value)}
            spellCheck={false}
          />
          <span className="hint">space-separated, appended to the java command</span>
        </div>
      </div>
      <div style={{ marginTop: 8 }}>
        <div className="setting-row">
          <div className="desc">
            <div className="t">Aikar GC flags</div>
            <div className="s">Recommended G1GC tuning for Paper-family servers</div>
          </div>
          <Toggle checked={aikar} onChange={setAikar} label="Aikar flags" />
        </div>
        <div className="setting-row">
          <div className="desc">
            <div className="t">Start with panel</div>
            <div className="s">Launch this server automatically when the panel starts</div>
          </div>
          <Toggle checked={autoStart} onChange={setAutoStart} label="autostart" />
        </div>
        <div className="setting-row">
          <div className="desc">
            <div className="t">Restart after crash</div>
            <div className="s">Relaunch automatically after an unexpected exit (max 3 in 10 min)</div>
          </div>
          <Toggle checked={autoRestart} onChange={setAutoRestart} label="auto-restart" />
        </div>
      </div>
      <dl className="kv" style={{ marginTop: 8 }}>
        <dt>Java</dt>
        <dd className="mono">{server.java || "java"}</dd>
        <dt>Jar</dt>
        <dd className="mono">{server.jar || "run.sh"}</dd>
        <dt>Directory</dt>
        <dd className="mono">{server.dir}</dd>
      </dl>
    </section>
  );
}

// Properties shown first, in gameplay-relevant order; the rest follow
// alphabetically.
const PROP_PRIORITY = [
  "motd", "max-players", "difficulty", "gamemode", "hardcore", "pvp",
  "view-distance", "simulation-distance", "spawn-protection", "allow-flight",
  "allow-nether", "enable-command-block", "online-mode", "white-list",
  "enforce-whitelist", "level-name", "level-seed", "server-port",
];

function PropertiesSection({ id, serverState }: { id: string; serverState: string }) {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [edits, setEdits] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ["mc-props", id],
    queryFn: () => api<{ properties: PropEntry[] }>(`/api/minecraft/${id}/properties`),
  });

  const ordered = useMemo(() => {
    const props = data?.properties ?? [];
    const rank = new Map(PROP_PRIORITY.map((k, i) => [k, i]));
    return [...props].sort((a, b) => {
      const ra = rank.get(a.key) ?? 1000;
      const rb = rank.get(b.key) ?? 1000;
      return ra !== rb ? ra - rb : a.key.localeCompare(b.key);
    });
  }, [data]);

  const dirtyKeys = Object.keys(edits).filter((k) => {
    const orig = data?.properties?.find((p) => p.key === k)?.value;
    return edits[k] !== orig;
  });

  async function save() {
    const changes: Record<string, string> = {};
    for (const k of dirtyKeys) changes[k] = edits[k];
    if (Object.keys(changes).length === 0) return;
    setSaving(true);
    try {
      await api(`/api/minecraft/${id}/properties`, { method: "PUT", body: { changes } });
      toast("ok", `saved ${Object.keys(changes).length} propert${Object.keys(changes).length === 1 ? "y" : "ies"}`);
      setEdits({});
      queryClient.invalidateQueries({ queryKey: ["mc-props", id] });
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  if (isLoading) {
    return (
      <section>
        <Spinner />
      </section>
    );
  }

  return (
    <section>
      <div className="row" style={{ marginBottom: 8 }}>
        <span className="label">server.properties</span>
        <span className="small faint">
          comments and order are preserved
          {serverState === "running" ? " · changes apply on restart" : ""}
        </span>
        {dirtyKeys.length > 0 && (
          <button className="btn btn-sm btn-primary right" onClick={save} disabled={saving}>
            {saving ? <Spinner size={12} /> : <Icon name="check" size={12} />}
            Save {dirtyKeys.length} change{dirtyKeys.length > 1 ? "s" : ""}
          </button>
        )}
      </div>
      <div className="props-grid">
        {ordered.map((p) => {
          const val = edits[p.key] ?? p.value;
          const dirty = dirtyKeys.includes(p.key);
          const isBool = p.value === "true" || p.value === "false";
          return (
            <div className="field" key={p.key}>
              <span className="label" style={dirty ? { color: "var(--accent-text)" } : undefined}>
                {p.key}
                {dirty ? " *" : ""}
              </span>
              {isBool ? (
                <select
                  className="select mono"
                  value={val}
                  onChange={(e) => setEdits((prev) => ({ ...prev, [p.key]: e.target.value }))}
                >
                  <option value="true">true</option>
                  <option value="false">false</option>
                </select>
              ) : (
                <input
                  className="input mono"
                  value={val}
                  onChange={(e) => setEdits((prev) => ({ ...prev, [p.key]: e.target.value }))}
                  spellCheck={false}
                />
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}
