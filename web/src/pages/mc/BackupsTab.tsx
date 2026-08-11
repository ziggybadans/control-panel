// Backups: list, create (job with streamed output), restore (typed confirm,
// server must be stopped), delete (typed confirm).

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "../../api/client";
import type { BackupInfo, Job, MCServer } from "../../api/types";
import { fmtBytes, fmtDateTime, fmtRelative } from "../../lib/format";
import { useJobs } from "../../state/live";
import { Spinner } from "../../ui/bits";
import { useConfirm } from "../../ui/Confirm";
import { Icon } from "../../ui/Icon";
import { useToast } from "../../ui/Toast";
import { JobPanel } from "../Storage";

export function BackupsTab({ id, server }: { id: string; server: MCServer }) {
  const toast = useToast();
  const confirm = useConfirm();
  const queryClient = useQueryClient();
  const jobs = useJobs();
  const [expandedJob, setExpandedJob] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["mc-backups", id],
    queryFn: () => api<{ backups: BackupInfo[] }>(`/api/minecraft/${id}/backups`),
    refetchInterval: 10_000,
  });

  const backupJob = jobs.find((j) => j.kind === "mc.backup" && j.target === id);
  const running = backupJob?.state === "running";
  const stopped = server.state === "stopped" || server.state === "crashed";

  // Refresh the archive list the moment a backup job finishes.
  const jobState = backupJob?.state;
  useEffect(() => {
    if (jobState === "done") {
      queryClient.invalidateQueries({ queryKey: ["mc-backups", id] });
    }
  }, [jobState, id, queryClient]);

  async function create() {
    try {
      const job = await api<Job>(`/api/minecraft/${id}/backups`, { method: "POST" });
      setExpandedJob(job.id);
      toast("ok", "backup started");
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "backup failed to start");
    }
  }

  async function restore(b: BackupInfo) {
    const ok = await confirm({
      title: `Restore ${b.name}`,
      target: b.name,
      typed: true,
      body: (
        <>
          The current server directory will be <b>replaced</b> with this backup from{" "}
          {fmtDateTime(b.createdAt)}. The replaced data is kept next to the backups
          (not deleted), but any progress since the backup will be gone from the live
          server.
        </>
      ),
      confirmLabel: "Restore backup",
    });
    if (!ok) return;
    try {
      await api(`/api/minecraft/${id}/backups/${encodeURIComponent(b.name)}/restore`, {
        method: "POST",
        confirm: b.name,
      });
      toast("ok", `restored ${b.name}`);
      queryClient.invalidateQueries({ queryKey: ["mc-backups", id] });
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "restore failed");
    }
  }

  async function remove(b: BackupInfo) {
    const ok = await confirm({
      title: `Delete backup ${b.name}`,
      target: b.name,
      typed: true,
      body: `This deletes the ${fmtBytes(b.sizeBytes)} archive permanently.`,
      confirmLabel: "Delete backup",
    });
    if (!ok) return;
    try {
      await api(`/api/minecraft/${id}/backups/${encodeURIComponent(b.name)}`, {
        method: "DELETE",
        confirm: b.name,
      });
      toast("ok", `deleted ${b.name}`);
      queryClient.invalidateQueries({ queryKey: ["mc-backups", id] });
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "delete failed");
    }
  }

  const backups = data?.backups ?? [];

  return (
    <div className="card-b" style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      <div className="row">
        <span className="small muted">
          Backups exclude logs and caches. While the server runs, world saves are
          paused around the archive step (save-off / save-on).
        </span>
        <button className="btn btn-primary right" onClick={create} disabled={running}>
          {running ? <Spinner size={13} /> : <Icon name="archive" size={13} />}
          Back up now
        </button>
      </div>

      {backupJob && (
        <JobPanel
          job={backupJob}
          expanded={expandedJob === backupJob.id || backupJob.state === "running"}
          onToggle={() => setExpandedJob(expandedJob === backupJob.id ? null : backupJob.id)}
        />
      )}

      {isLoading ? (
        <Spinner />
      ) : backups.length === 0 ? (
        <div className="small faint">No backups yet.</div>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>Archive</th>
              <th className="r">Size</th>
              <th className="r">Created</th>
              <th className="r" style={{ width: 190 }}>
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {backups.map((b) => (
              <tr key={b.name}>
                <td className="mono">{b.name}</td>
                <td className="r num">{fmtBytes(b.sizeBytes)}</td>
                <td className="r num" title={fmtDateTime(b.createdAt)}>
                  {fmtRelative(b.createdAt)}
                </td>
                <td className="r">
                  <div className="row" style={{ justifyContent: "flex-end", gap: 4 }}>
                    <button
                      className="btn btn-sm"
                      onClick={() => restore(b)}
                      disabled={!stopped}
                      title={stopped ? "restore this backup" : "stop the server first"}
                    >
                      <Icon name="download" size={12} />
                      Restore
                    </button>
                    <button className="btn btn-sm btn-danger" onClick={() => remove(b)}>
                      <Icon name="trash" size={12} />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {!stopped && backups.length > 0 && (
        <div className="small faint">Restoring requires the server to be stopped.</div>
      )}
    </div>
  );
}
