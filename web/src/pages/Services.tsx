// Services page: allowlisted systemd units with lifecycle controls and an
// expandable journal drawer per unit.

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { Service, ServicesResponse } from "../api/types";
import { fmtBytes, fmtUptimeSince } from "../lib/format";
import { useServices } from "../state/live";
import { Card, ServiceBadge, Spinner } from "../ui/bits";
import { useConfirm } from "../ui/Confirm";
import { Icon } from "../ui/Icon";
import { useToast } from "../ui/Toast";

export function ServicesPage() {
  const live = useServices();
  const { data } = useQuery({
    queryKey: ["services-meta"],
    queryFn: () => api<ServicesResponse>("/api/services"),
  });
  const services = live.length > 0 ? live : (data?.services ?? []);
  const allowActions = data?.allowActions ?? true;
  const [openLogs, setOpenLogs] = useState<string | null>(null);

  const present = services.filter((s) => s.loadState !== "not-found");
  const missing = services.filter((s) => s.loadState === "not-found");

  return (
    <div className="page">
      <Card
        title={`Configured units · ${present.length}`}
        actions={
          !allowActions ? (
            <span className="badge neutral">
              <Icon name="lock" size={11} />
              read-only
            </span>
          ) : undefined
        }
        flush
      >
        <table className="table">
          <thead>
            <tr>
              <th>Unit</th>
              <th>State</th>
              <th className="r">Since</th>
              <th className="r">PID</th>
              <th className="r">Memory</th>
              <th className="r">Boot</th>
              <th className="r" style={{ width: 220 }}>
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {present.map((s) => (
              <ServiceRow
                key={s.unit}
                svc={s}
                allowActions={allowActions}
                logsOpen={openLogs === s.unit}
                onToggleLogs={() => setOpenLogs(openLogs === s.unit ? null : s.unit)}
              />
            ))}
          </tbody>
        </table>
      </Card>
      {missing.length > 0 && (
        <div className="small faint">
          Not installed on this host: {missing.map((s) => s.unit).join(", ")}
        </div>
      )}
    </div>
  );
}

function ServiceRow({
  svc,
  allowActions,
  logsOpen,
  onToggleLogs,
}: {
  svc: Service;
  allowActions: boolean;
  logsOpen: boolean;
  onToggleLogs: () => void;
}) {
  const confirm = useConfirm();
  const toast = useToast();
  const [busy, setBusy] = useState<string | null>(null);

  const isActive = svc.activeState === "active" || svc.activeState === "activating";

  async function action(verb: "start" | "stop" | "restart") {
    if (verb !== "start") {
      const ok = await confirm({
        title: `${verb === "stop" ? "Stop" : "Restart"} ${svc.unit}`,
        target: svc.unit,
        body:
          verb === "stop"
            ? `Anything using ${svc.description || svc.unit} will be interrupted until it is started again.`
            : `${svc.description || svc.unit} will briefly go down while it restarts.`,
        confirmLabel: verb === "stop" ? "Stop service" : "Restart service",
      });
      if (!ok) return;
    }
    setBusy(verb);
    try {
      await api(`/api/services/${encodeURIComponent(svc.unit)}/${verb}`, {
        method: "POST",
        confirm: verb !== "start" ? svc.unit : undefined,
      });
      toast("ok", `${svc.unit}: ${verb} issued`);
    } catch (e) {
      toast("error", e instanceof Error ? e.message : `${verb} failed`);
    } finally {
      setBusy(null);
    }
  }

  return (
    <>
      <tr>
        <td>
          <div style={{ fontWeight: 550 }}>{svc.unit}</div>
          <div className="small faint">{svc.description}</div>
        </td>
        <td>
          <ServiceBadge active={svc.activeState} sub={svc.subState} />
        </td>
        <td className="r num small muted">{svc.since ? fmtUptimeSince(svc.since) : "—"}</td>
        <td className="r num small muted">{svc.pid || "—"}</td>
        <td className="r num small muted">{svc.memBytes ? fmtBytes(svc.memBytes) : "—"}</td>
        <td className="r small muted">{svc.enabled || "—"}</td>
        <td className="r">
          <div className="row" style={{ justifyContent: "flex-end", gap: 4 }}>
            {busy ? (
              <Spinner size={13} />
            ) : (
              allowActions && (
                <>
                  {!isActive && (
                    <button className="btn btn-sm" onClick={() => action("start")}>
                      <Icon name="play" size={12} />
                      Start
                    </button>
                  )}
                  {isActive && (
                    <>
                      <button className="btn btn-sm" onClick={() => action("restart")}>
                        <Icon name="restart" size={12} />
                        Restart
                      </button>
                      <button className="btn btn-sm btn-danger" onClick={() => action("stop")}>
                        <Icon name="stop" size={12} />
                        Stop
                      </button>
                    </>
                  )}
                </>
              )
            )}
            <button
              className={logsOpen ? "btn btn-sm btn-primary" : "btn btn-sm btn-ghost"}
              onClick={onToggleLogs}
              title="journal"
            >
              <Icon name="terminal" size={12} />
              Logs
            </button>
          </div>
        </td>
      </tr>
      {logsOpen && (
        <tr>
          <td colSpan={7} style={{ padding: 0, background: "var(--surface-2)" }}>
            <LogDrawer unit={svc.unit} />
          </td>
        </tr>
      )}
    </>
  );
}

function LogDrawer({ unit }: { unit: string }) {
  const { data, isLoading, refetch, isFetching } = useQuery({
    queryKey: ["service-logs", unit],
    queryFn: () =>
      api<{ lines: string[] }>(`/api/services/${encodeURIComponent(unit)}/logs?lines=200`),
    refetchInterval: 5000,
  });
  return (
    <div style={{ padding: "8px 12px" }}>
      <div className="row small muted" style={{ marginBottom: 6 }}>
        <span>journalctl -u {unit} -n 200</span>
        <button className="btn btn-ghost btn-sm right" onClick={() => refetch()}>
          {isFetching ? <Spinner size={11} /> : <Icon name="refresh" size={12} />}
          Refresh
        </button>
      </div>
      <div className="job-output">
        {isLoading ? "loading…" : (data?.lines ?? []).join("\n") || "no journal entries"}
      </div>
    </div>
  );
}
