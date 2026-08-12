// New-server setup: pick flavor + version (fetched live), memory, port,
// MOTD, EULA — provisioning runs as a job with streamed download progress.

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
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
  const [spec, setSpec] = useState<MCCreateSpec>({
    id: "",
    flavor: "paper",
    version: "",
    mem: "4G",
    port: 25565,
    motd: "A Minecraft Server",
    acceptEula: false,
  });
  const [jobId, setJobId] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const { data: versions, isLoading: versionsLoading } = useQuery({
    queryKey: ["mc-versions", spec.flavor],
    queryFn: () =>
      api<{ versions: string[] }>(`/api/minecraft/meta/versions?flavor=${spec.flavor}`),
    staleTime: 3600_000,
  });

  const createJob = jobId ? jobs.find((j) => j.id === jobId) : undefined;
  const version = spec.version || versions?.versions?.[0] || "";
  const idValid = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$/.test(spec.id);
  const ready = idValid && version && spec.acceptEula && !submitting;

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
              </div>

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
            </>
          )}
        </div>
        <div className="modal-f">
          <button className="btn" onClick={onClose} disabled={submitting && createJob?.state === "running"}>
            {createJob?.state === "done" ? "Close" : "Cancel"}
          </button>
          {!createJob && (
            <button className="btn btn-primary" disabled={!ready} onClick={create}>
              {submitting ? <Spinner size={13} /> : "Create server"}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
