// New-server setup: download official server software (flavor + version
// fetched live) or import an existing server from a zip. Both run as jobs
// with streamed progress.

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api, apiUpload } from "../../api/client";
import type { Job, MCCreateSpec } from "../../api/types";
import { useJobs } from "../../state/live";
import { Spinner, Toggle } from "../../ui/bits";
import { useToast } from "../../ui/Toast";
import { JobPanel } from "../Storage";

const FLAVORS = [
  { id: "paper", label: "Paper", hint: "recommended — fast, plugin support" },
  { id: "purpur", label: "Purpur", hint: "Paper fork with extra knobs" },
  { id: "fabric", label: "Fabric", hint: "lightweight mod loader" },
  { id: "vanilla", label: "Vanilla", hint: "unmodified Mojang server" },
];

export function SetupModal({ onClose }: { onClose: () => void }) {
  const toast = useToast();
  const jobs = useJobs();
  const [mode, setMode] = useState<"create" | "import">("create");
  const [spec, setSpec] = useState<MCCreateSpec>({
    id: "",
    flavor: "paper",
    version: "",
    mem: "4G",
    port: 25565,
    motd: "A Minecraft Server",
    acceptEula: false,
  });
  const [zipFile, setZipFile] = useState<File | null>(null);
  const [jobId, setJobId] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const { data: versions, isLoading: versionsLoading } = useQuery({
    queryKey: ["mc-versions", spec.flavor],
    queryFn: () =>
      api<{ versions: string[] }>(`/api/minecraft/meta/versions?flavor=${spec.flavor}`),
    staleTime: 3600_000,
    enabled: mode === "create",
  });

  const createJob = jobId ? jobs.find((j) => j.id === jobId) : undefined;
  const version = spec.version || versions?.versions?.[0] || "";
  const idValid = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$/.test(spec.id);
  const ready =
    !submitting &&
    idValid &&
    (mode === "create" ? Boolean(version) && spec.acceptEula : zipFile !== null);

  async function create() {
    setSubmitting(true);
    try {
      const job = await api<Job>("/api/minecraft/create", {
        method: "POST",
        body: { ...spec, version },
      });
      setJobId(job.id);
      toast("ok", `provisioning ${spec.id}`);
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "create failed");
      setSubmitting(false);
    }
  }

  async function importZip() {
    if (!zipFile) return;
    setSubmitting(true);
    try {
      const job = await apiUpload<Job>(
        `/api/minecraft/import?id=${encodeURIComponent(spec.id)}&mem=${encodeURIComponent(spec.mem)}`,
        [zipFile],
      );
      setJobId(job.id);
      toast("ok", `importing ${spec.id}`);
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "import failed");
      setSubmitting(false);
    }
  }

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && !submitting && onClose()}>
      <div className="modal wide">
        <div className="modal-h">New Minecraft server</div>
        <div className="modal-b" style={{ gap: 14 }}>
          {createJob ? (
            <>
              <JobPanel job={createJob} expanded onToggle={() => {}} />
              {createJob.state === "done" && (
                <div className="ok-text small">
                  Server created. It appears in the list — accept nothing further;
                  EULA and configuration are already in place.
                </div>
              )}
              {createJob.state === "failed" && (
                <div className="crit-text small">{createJob.err}</div>
              )}
            </>
          ) : (
            <>
              <div className="choice-row" role="tablist" aria-label="setup mode">
                <button
                  role="tab"
                  aria-selected={mode === "create"}
                  className={`choice ${mode === "create" ? "selected" : ""}`}
                  onClick={() => setMode("create")}
                >
                  Download new
                </button>
                <button
                  role="tab"
                  aria-selected={mode === "import"}
                  className={`choice ${mode === "import" ? "selected" : ""}`}
                  onClick={() => setMode("import")}
                >
                  Import existing (zip)
                </button>
              </div>

              <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: 12 }}>
                <div className="field">
                  <span className="label">Server id (folder name)</span>
                  <input
                    className="input mono"
                    placeholder="e.g. skyblock"
                    value={spec.id}
                    onChange={(e) => setSpec({ ...spec, id: e.target.value.trim() })}
                    autoFocus
                    spellCheck={false}
                  />
                  {spec.id && !idValid && (
                    <span className="hint crit-text">letters, digits, - _ . only</span>
                  )}
                </div>
                {mode === "create" ? (
                  <div className="field">
                    <span className="label">Version</span>
                    <select
                      className="select mono"
                      value={version}
                      onChange={(e) => setSpec({ ...spec, version: e.target.value })}
                    >
                      {versionsLoading && <option>loading…</option>}
                      {(versions?.versions ?? []).slice(0, 30).map((v) => (
                        <option key={v} value={v}>
                          {v}
                        </option>
                      ))}
                    </select>
                  </div>
                ) : (
                  <div className="field">
                    <span className="label">Memory</span>
                    <input
                      className="input mono"
                      value={spec.mem}
                      onChange={(e) => setSpec({ ...spec, mem: e.target.value })}
                      spellCheck={false}
                    />
                  </div>
                )}
              </div>

              {mode === "import" && (
                <>
                  <div className="field">
                    <span className="label">Server archive (.zip)</span>
                    <input
                      className="input"
                      type="file"
                      accept=".zip"
                      onChange={(e) => setZipFile(e.target.files?.[0] ?? null)}
                    />
                    <span className="hint">
                      A zip of the server folder or its contents (a single
                      top-level folder is flattened automatically). The server
                      jar is auto-detected; if that fails, pick one afterwards
                      under Settings → Server jar.
                    </span>
                  </div>
                  <div className="small faint">
                    Extraction is confined to the new server directory and
                    size-capped. Existing world data, configs, and mods are kept
                    exactly as archived — nothing is modified.
                  </div>
                </>
              )}

              {mode === "create" && (
              <div className="field">
                <span className="label">Software</span>
                <div className="choice-row wrap">
                  {FLAVORS.map((f) => (
                    <button
                      key={f.id}
                      className={`choice ${spec.flavor === f.id ? "selected" : ""}`}
                      title={f.hint}
                      onClick={() => setSpec({ ...spec, flavor: f.id, version: "" })}
                    >
                      {f.label}
                    </button>
                  ))}
                </div>
                <span className="hint">
                  {FLAVORS.find((f) => f.id === spec.flavor)?.hint}
                </span>
              </div>
              )}

              {mode === "create" && (
              <div className="grid" style={{ gridTemplateColumns: "1fr 1fr 2fr", gap: 12 }}>
                <div className="field">
                  <span className="label">Memory</span>
                  <input
                    className="input mono"
                    value={spec.mem}
                    onChange={(e) => setSpec({ ...spec, mem: e.target.value })}
                    spellCheck={false}
                  />
                </div>
                <div className="field">
                  <span className="label">Port</span>
                  <input
                    className="input mono"
                    type="number"
                    value={spec.port}
                    onChange={(e) => setSpec({ ...spec, port: Number(e.target.value) })}
                  />
                </div>
                <div className="field">
                  <span className="label">MOTD</span>
                  <input
                    className="input"
                    value={spec.motd}
                    onChange={(e) => setSpec({ ...spec, motd: e.target.value })}
                  />
                </div>
              </div>
              )}

              {mode === "create" && (
              <div className="setting-row" style={{ borderBottom: "none", padding: 0 }}>
                <div className="desc">
                  <div className="t">Accept the Minecraft EULA</div>
                  <div className="s">
                    Required to start.{" "}
                    <a href="https://aka.ms/MinecraftEULA" target="_blank" rel="noreferrer">
                      Read the EULA
                    </a>
                  </div>
                </div>
                <Toggle
                  checked={spec.acceptEula}
                  onChange={(v) => setSpec({ ...spec, acceptEula: v })}
                  label="accept eula"
                />
              </div>
              )}
            </>
          )}
        </div>
        <div className="modal-f">
          <button className="btn" onClick={onClose} disabled={submitting && createJob?.state === "running"}>
            {createJob?.state === "done" ? "Close" : "Cancel"}
          </button>
          {!createJob && (
            <button
              className="btn btn-primary"
              disabled={!ready}
              onClick={mode === "create" ? create : importZip}
            >
              {submitting ? (
                <Spinner size={13} />
              ) : mode === "create" ? (
                "Create server"
              ) : (
                "Upload & import"
              )}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
