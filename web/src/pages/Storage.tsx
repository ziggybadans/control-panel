// Storage page: mergerfs pool with branches, physical disks with SMART,
// other mounts, and the snapraid panel with job execution.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { Disk, Job, SnapraidInfo, StorageOverview } from "../api/types";
import { fmtBytes, fmtPct, fmtTemp } from "../lib/format";
import { useJobs } from "../state/live";
import { CapacityBar, Card, EmptyState, Spinner } from "../ui/bits";
import { useConfirm } from "../ui/Confirm";
import { Icon } from "../ui/Icon";
import { useToast } from "../ui/Toast";

export function StoragePage() {
  const { data, isLoading } = useQuery({
    queryKey: ["storage"],
    queryFn: () => api<StorageOverview>("/api/storage"),
    refetchInterval: 15_000,
  });

  if (isLoading) {
    return (
      <div className="page">
        <div className="empty">
          <Spinner size={20} />
        </div>
      </div>
    );
  }

  const pools = data?.pools ?? [];
  const disks = data?.disks ?? [];
  const mounts = data?.mounts ?? [];

  return (
    <div className="page">
      {pools.map((pool) => (
        <Card
          key={pool.mount}
          title={
            <>
              Pool · <span className="mono">{pool.mount}</span>
            </>
          }
          actions={<span className="small faint">{pool.fsType}</span>}
        >
          <div className="row" style={{ alignItems: "baseline", marginBottom: 6 }}>
            <span style={{ fontSize: 20, fontWeight: 650 }} className="num">
              {fmtBytes(pool.used)}
            </span>
            <span className="muted small">
              of {fmtBytes(pool.total)} used · {fmtBytes(pool.total - pool.used)} free
            </span>
            <span className="right num" style={{ fontWeight: 600 }}>
              {fmtPct((pool.used / pool.total) * 100)}
            </span>
          </div>
          <CapacityBar used={pool.used} total={pool.total} thick />
          <div className="pool-branches">
            {(pool.branches ?? []).map((b) => (
              <div key={b.path} className="branch-tile">
                <div className="row small">
                  <span className="mono truncate">{b.path}</span>
                  <span className="right faint">{b.device}</span>
                </div>
                <CapacityBar used={b.used} total={b.total} />
                <div className="row small muted num">
                  <span>{fmtBytes(b.used)}</span>
                  <span className="right">{fmtBytes(b.total)}</span>
                </div>
              </div>
            ))}
          </div>
        </Card>
      ))}
      {pools.length === 0 && (
        <Card title="Pool">
          <EmptyState
            icon="storage"
            title="No mergerfs pool detected"
            hint="Pools are auto-detected from /proc/mounts, or list them under storage.pools in config.yaml."
          />
        </Card>
      )}

      <Card title="Physical disks" flush>
        <DiskTable disks={disks} />
      </Card>

      <div className="grid" style={{ gridTemplateColumns: "1fr 1fr" }}>
        <Card title="Other mounts" flush>
          <table className="table">
            <thead>
              <tr>
                <th>Mount</th>
                <th>Device</th>
                <th>FS</th>
                <th className="r">Used</th>
                <th className="r">Size</th>
                <th style={{ width: 110 }}></th>
              </tr>
            </thead>
            <tbody>
              {mounts.map((m) => (
                <tr key={m.mount}>
                  <td className="mono">{m.mount}</td>
                  <td className="mono small muted">{m.device}</td>
                  <td className="small muted">{m.fsType}</td>
                  <td className="r num">{fmtBytes(m.used)}</td>
                  <td className="r num">{fmtBytes(m.total)}</td>
                  <td>
                    <CapacityBar used={m.used} total={m.total} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
        <SnapraidCard />
      </div>
    </div>
  );
}

function DiskTable({ disks }: { disks: Disk[] }) {
  return (
    <table className="table">
      <thead>
        <tr>
          <th>Device</th>
          <th>Model</th>
          <th className="r">Size</th>
          <th>Type</th>
          <th className="r">Temp</th>
          <th className="r">Power-on</th>
          <th>SMART</th>
        </tr>
      </thead>
      <tbody>
        {disks.map((d) => (
          <tr key={d.name}>
            <td className="mono">{d.name}</td>
            <td>
              <div>{d.model || "—"}</div>
              {d.serial && <div className="small faint mono">{d.serial}</div>}
            </td>
            <td className="r num">{fmtBytes(d.sizeBytes, 1)}</td>
            <td className="small muted">{d.rotational ? "HDD" : "SSD"}</td>
            <td className={`r num ${d.tempC && d.tempC >= (d.rotational ? 45 : 60) ? "warn-text" : ""}`}>
              {fmtTemp(d.tempC)}
            </td>
            <td className="r num small muted">
              {d.smart.powerOnHours
                ? `${Math.round(d.smart.powerOnHours / 24 / 365 * 10) / 10}y`
                : "—"}
            </td>
            <td>
              <SmartCell d={d} />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function SmartCell({ d }: { d: Disk }) {
  const s = d.smart;
  if (!s.available) return <span className="badge neutral">n/a</span>;
  const issues: string[] = [];
  if (s.reallocated) issues.push(`${s.reallocated} reallocated`);
  if (s.pendingSectors) issues.push(`${s.pendingSectors} pending`);
  if (s.crcErrors) issues.push(`${s.crcErrors} CRC`);
  if (s.mediaErrors) issues.push(`${s.mediaErrors} media err`);
  if (s.healthy === false) {
    return (
      <span className="badge crit" title={issues.join(", ")}>
        <Icon name="warning" size={11} />
        FAILING
      </span>
    );
  }
  if (issues.length > 0) {
    return (
      <span className="badge warn" title="SMART attributes worth watching">
        {issues.join(" · ")}
      </span>
    );
  }
  if (s.percentUsed !== undefined && s.percentUsed > 0) {
    return (
      <span className="badge ok" title="NVMe wear level">
        healthy · {s.percentUsed}% worn
      </span>
    );
  }
  return <span className="badge ok">healthy</span>;
}

// --- SnapRAID ---------------------------------------------------------------

function SnapraidCard() {
  const confirm = useConfirm();
  const toast = useToast();
  const jobs = useJobs();
  const queryClient = useQueryClient();
  const [viewJob, setViewJob] = useState<string | null>(null);

  const { data: info } = useQuery({
    queryKey: ["snapraid"],
    queryFn: () => api<SnapraidInfo>("/api/storage/snapraid"),
  });

  const snapJob = jobs.find((j) => j.kind.startsWith("snapraid."));
  const running = snapJob?.state === "running" ? snapJob : undefined;

  async function run(op: string) {
    if (op === "sync" || op === "scrub") {
      const ok = await confirm({
        title: `Run snapraid ${op}`,
        target: op,
        typed: true,
        body:
          op === "sync"
            ? "sync updates parity to match the current data. Deleted or modified files become unrecoverable from parity once synced."
            : "scrub verifies parity by reading data across all drives. It is safe but I/O heavy and can take hours.",
        confirmLabel: `Run ${op}`,
      });
      if (!ok) return;
    }
    try {
      const job = await api<Job>(`/api/storage/snapraid/${op}`, {
        method: "POST",
        confirm: op === "sync" || op === "scrub" ? op : undefined,
      });
      setViewJob(job.id);
      toast("ok", `snapraid ${op} started`);
      queryClient.invalidateQueries({ queryKey: ["jobs"] });
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "failed to start job");
    }
  }

  return (
    <Card
      title="SnapRAID"
      actions={
        info?.configured ? (
          <div className="row">
            <button className="btn btn-sm" disabled={!!running} onClick={() => run("status")}>
              Status
            </button>
            <button className="btn btn-sm" disabled={!!running} onClick={() => run("diff")}>
              Diff
            </button>
            <button className="btn btn-sm" disabled={!!running} onClick={() => run("scrub")}>
              Scrub
            </button>
            <button className="btn btn-sm btn-primary" disabled={!!running} onClick={() => run("sync")}>
              Sync
            </button>
          </div>
        ) : undefined
      }
    >
      {!info ? (
        <Spinner />
      ) : !info.configured ? (
        <EmptyState
          icon="services"
          title="SnapRAID is not configured"
          hint={
            <>
              Point <span className="mono">storage.snapraid.config</span> at your
              snapraid.conf once you add parity. The panel will pick it up on restart.
            </>
          }
        />
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
          <dl className="kv">
            <dt>Config</dt>
            <dd className="mono">{info.configPath}</dd>
            <dt>Parity</dt>
            <dd className="mono">{info.parity?.join(", ") || "—"}</dd>
            <dt>Data disks</dt>
            <dd>{info.dataDisks?.length ?? 0}</dd>
            <dt>Binary</dt>
            <dd>{info.installed ? "installed" : <span className="crit-text">not found</span>}</dd>
          </dl>
          {snapJob && (
            <JobPanel
              job={snapJob}
              expanded={viewJob === snapJob.id || snapJob.state === "running"}
              onToggle={() => setViewJob(viewJob === snapJob.id ? null : snapJob.id)}
            />
          )}
        </div>
      )}
    </Card>
  );
}

/** Job status line + live output (polls while running). */
export function JobPanel({
  job,
  expanded,
  onToggle,
}: {
  job: Job;
  expanded: boolean;
  onToggle: () => void;
}) {
  const toast = useToast();
  // job.state in the key ensures a final refetch when the job completes,
  // so the output panel never shows a stale mid-run snapshot.
  const { data: full } = useQuery({
    queryKey: ["job", job.id, job.state],
    queryFn: () => api<Job>(`/api/jobs/${job.id}`),
    refetchInterval: job.state === "running" ? 1000 : false,
    enabled: expanded,
  });

  const badge =
    job.state === "running" ? (
      <span className="badge accent">
        <Spinner size={10} /> running
      </span>
    ) : job.state === "done" ? (
      <span className="badge ok">done</span>
    ) : job.state === "canceled" ? (
      <span className="badge neutral">canceled</span>
    ) : (
      <span className="badge crit" title={job.err}>
        failed
      </span>
    );

  return (
    <div>
      <div className="row small">
        <button className="btn btn-ghost btn-sm" onClick={onToggle}>
          <Icon name={expanded ? "chevronDown" : "chevronRight"} size={13} />
          <span className="mono">{job.kind}</span>
        </button>
        {badge}
        {job.state === "running" && (
          <button
            className="btn btn-sm btn-danger right"
            onClick={() =>
              api(`/api/jobs/${job.id}/cancel`, { method: "POST" }).catch((e) =>
                toast("error", e.message),
              )
            }
          >
            Cancel
          </button>
        )}
      </div>
      {expanded && (
        <div className="reveal" style={{ marginTop: 6 }}>
          <div className="job-output">
            {(full?.output ?? []).join("\n") || "waiting for output…"}
            {job.err && `\n\nerror: ${job.err}`}
          </div>
        </div>
      )}
    </div>
  );
}
