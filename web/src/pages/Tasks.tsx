// Scheduled tasks: allowlisted panel actions (Minecraft backups/restarts/
// commands, service restarts, snapraid runs) on interval/daily/weekly
// recurrences. Executions land in the audit log with "scheduler" as the
// source; job output shows up in the usual job stream.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { Schedule, ScheduleAction, SchedulesResponse } from "../api/types";
import { fmtDateTime, fmtRelative } from "../lib/format";
import { TopbarActions } from "../shell/Layout";
import { useMCServers, useServices } from "../state/live";
import { EmptyState, Spinner, Toggle } from "../ui/bits";
import { useConfirm } from "../ui/Confirm";
import { Icon } from "../ui/Icon";
import { useToast } from "../ui/Toast";

const ACTION_LABELS: Record<ScheduleAction, string> = {
  "mc.backup": "Back up Minecraft server",
  "mc.restart": "Restart Minecraft server",
  "mc.command": "Run Minecraft command",
  "service.restart": "Restart service",
  "snapraid.sync": "snapraid sync",
  "snapraid.scrub": "snapraid scrub",
};

export function describeAction(s: Schedule): string {
  switch (s.action) {
    case "mc.backup":
      return `Back up ${s.server}${s.keep ? ` · keep ${s.keep}` : ""}`;
    case "mc.restart":
      return `Restart ${s.server}`;
    case "mc.command":
      return `${s.server}: ${s.command}`;
    case "service.restart":
      return `Restart ${s.unit}`;
    default:
      return s.action;
  }
}

export function describeWhen(s: Schedule): string {
  if (s.every) return `every ${s.every}`;
  if (s.daily) return `daily at ${s.daily}`;
  if (s.weekly) return `weekly ${s.weekly}`;
  return "—";
}

/**
 * The schedule table with its row actions (toggle, run-now, edit, delete).
 * Shared between the Tasks page and each Minecraft server's Tasks tab.
 */
export function ScheduleTable({
  schedules,
  onEdit,
}: {
  schedules: Schedule[];
  onEdit: (s: Schedule) => void;
}) {
  const toast = useToast();
  const confirm = useConfirm();
  const qc = useQueryClient();

  function refresh() {
    qc.invalidateQueries({ queryKey: ["schedules"] });
  }

  async function toggle(s: Schedule, enabled: boolean) {
    try {
      await api(`/api/schedules/${s.id}`, { method: "PUT", body: { ...s, enabled } });
      refresh();
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "update failed");
    }
  }

  async function runNow(s: Schedule) {
    try {
      await api(`/api/schedules/${s.id}/run`, { method: "POST" });
      toast("ok", `${s.name}: running`);
      setTimeout(refresh, 1500);
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "run failed");
    }
  }

  async function remove(s: Schedule) {
    const ok = await confirm({
      title: `Delete task ${s.name}`,
      target: s.name,
      body: (
        <>
          The scheduled task <b>{s.name}</b> ({describeAction(s)},{" "}
          {describeWhen(s)}) will be removed. Nothing it created (backups etc.)
          is touched.
        </>
      ),
      confirmLabel: "Delete",
    });
    if (!ok) return;
    try {
      await api(`/api/schedules/${s.id}`, { method: "DELETE" });
      toast("ok", `deleted ${s.name}`);
      refresh();
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "delete failed");
    }
  }

  return (
    <table className="table">
      <thead>
        <tr>
          <th>Name</th>
          <th>Does</th>
          <th>When</th>
          <th>Last run</th>
          <th>Next run</th>
          <th className="r">Enabled</th>
          <th className="r" style={{ width: 130 }}></th>
        </tr>
      </thead>
      <tbody>
        {schedules.map((s) => (
          <tr key={s.id}>
            <td style={{ fontWeight: 550 }}>{s.name}</td>
            <td className="small muted">{describeAction(s)}</td>
            <td className="small muted num">{describeWhen(s)}</td>
            <td className="small">
              {s.lastRun ? (
                <span title={`${fmtDateTime(s.lastRun)} — ${s.lastResult ?? ""}`}>
                  <span
                    className={`dot ${s.lastResult?.startsWith("error") ? "crit" : "ok"}`}
                    style={{ marginRight: 6 }}
                  />
                  <span className="muted">{fmtRelative(s.lastRun)}</span>
                </span>
              ) : (
                <span className="faint">never</span>
              )}
            </td>
            <td className="small muted num" title={fmtDateTime(s.nextRun)}>
              {s.enabled ? fmtRelative(s.nextRun) : "—"}
            </td>
            <td className="r">
              <Toggle
                checked={s.enabled}
                onChange={(v) => void toggle(s, v)}
                label={`enable ${s.name}`}
              />
            </td>
            <td className="r">
              <div className="row" style={{ justifyContent: "flex-end", gap: 2 }}>
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={() => void runNow(s)}
                  title="Run once now"
                >
                  Run
                </button>
                <button
                  className="btn btn-ghost btn-sm btn-icon"
                  onClick={() => onEdit(s)}
                  title="Edit"
                >
                  <Icon name="edit" size={12} />
                </button>
                <button
                  className="btn btn-ghost btn-sm btn-icon crit-text"
                  onClick={() => void remove(s)}
                  title="Delete"
                >
                  <Icon name="trash" size={12} />
                </button>
              </div>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function TasksPage() {
  const qc = useQueryClient();
  const [editing, setEditing] = useState<Schedule | "new" | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["schedules"],
    queryFn: () => api<SchedulesResponse>("/api/schedules"),
    refetchInterval: 15_000,
  });
  const schedules = data?.schedules ?? [];

  return (
    <div className="page">
      <TopbarActions>
        <button className="btn btn-sm btn-primary" onClick={() => setEditing("new")}>
          <Icon name="plus" size={12} />
          New task
        </button>
      </TopbarActions>

      {isLoading ? (
        <Spinner />
      ) : schedules.length === 0 ? (
        <EmptyState
          icon="clock"
          title="No scheduled tasks"
          hint="Automate Minecraft backups, service restarts, snapraid runs, or console commands on a schedule. Tasks only run panel actions — never arbitrary commands."
        />
      ) : (
        <div className="card">
          <div className="card-b flush">
            <ScheduleTable schedules={schedules} onEdit={(s) => setEditing(s)} />
          </div>
        </div>
      )}

      <div className="small faint">
        Failed runs retry twice at 5-minute intervals before waiting for the
        next occurrence. Every execution is recorded in the{" "}
        <a href="#/activity">audit log</a>. Minecraft-specific tasks also
        appear on each server's Tasks tab.
      </div>

      {editing && (
        <TaskEditor
          initial={editing === "new" ? null : editing}
          onClose={(changed) => {
            setEditing(null);
            if (changed) qc.invalidateQueries({ queryKey: ["schedules"] });
          }}
        />
      )}
    </div>
  );
}

// --- editor -----------------------------------------------------------------

type WhenType = "every" | "daily" | "weekly";

const WEEKDAYS = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"];

export function TaskEditor({
  initial,
  onClose,
  preset,
  lockServer,
}: {
  initial: Schedule | null;
  onClose: (changed: boolean) => void;
  /** Prefills for the "new" form (used by the auto-backup shortcut). */
  preset?: Partial<Schedule>;
  /** Pin the task to one Minecraft server (per-server Tasks tab). */
  lockServer?: string;
}) {
  const toast = useToast();
  const mcServers = useMCServers();
  const services = useServices();
  const base = initial ?? {
    id: "",
    name: "",
    enabled: true,
    daily: "04:00",
    action: (lockServer ? "mc.command" : "mc.backup") as ScheduleAction,
    server: lockServer ?? mcServers[0]?.id,
    keep: 7,
    nextRun: 0,
    ...preset,
  };

  const [name, setName] = useState(base.name);
  const [action, setAction] = useState<ScheduleAction>(base.action);
  const [server, setServer] = useState(
    lockServer ?? base.server ?? mcServers[0]?.id ?? "",
  );
  const [unit, setUnit] = useState(base.unit ?? "");
  const [command, setCommand] = useState(base.command ?? "");
  const [keep, setKeep] = useState(base.keep ?? 0);
  const [onlyIfRunning, setOnlyIfRunning] = useState(base.onlyIfRunning ?? false);
  const [enabled, setEnabled] = useState(base.enabled);
  const [whenType, setWhenType] = useState<WhenType>(
    base.every ? "every" : base.weekly ? "weekly" : "daily",
  );
  const [every, setEvery] = useState(base.every ?? "6h");
  const [dailyTime, setDailyTime] = useState(base.daily ?? "04:00");
  const [weeklyDay, setWeeklyDay] = useState(base.weekly?.split(" ")[0] ?? "sun");
  const [weeklyTime, setWeeklyTime] = useState(base.weekly?.split(" ")[1] ?? "04:00");
  const [saving, setSaving] = useState(false);

  const isMC = action.startsWith("mc.");

  async function save() {
    const body: Schedule = {
      id: base.id,
      name: name.trim(),
      enabled,
      action,
      nextRun: 0,
      ...(whenType === "every" ? { every } : {}),
      ...(whenType === "daily" ? { daily: dailyTime } : {}),
      ...(whenType === "weekly" ? { weekly: `${weeklyDay} ${weeklyTime}` } : {}),
      ...(isMC ? { server } : {}),
      ...(action === "service.restart" ? { unit } : {}),
      ...(action === "mc.command" ? { command } : {}),
      ...(action === "mc.backup" ? { keep, onlyIfRunning } : {}),
      ...(action === "mc.command" ? { onlyIfRunning } : {}),
    };
    setSaving(true);
    try {
      if (initial) {
        await api(`/api/schedules/${initial.id}`, { method: "PUT", body });
      } else {
        await api("/api/schedules", { method: "POST", body });
      }
      toast("ok", `${body.name} saved`);
      onClose(true);
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "save failed");
      setSaving(false);
    }
  }

  return (
    <div
      className="modal-overlay"
      onMouseDown={(e) => e.target === e.currentTarget && onClose(false)}
    >
      <div className="modal wide">
        <div className="modal-h">{initial ? `Edit task — ${initial.name}` : "New scheduled task"}</div>
        <div className="modal-b" style={{ gap: 14 }}>
          <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: 12 }}>
            <div className="field">
              <span className="label">Name</span>
              <input
                className="input"
                placeholder="e.g. Nightly survival backup"
                value={name}
                maxLength={60}
                onChange={(e) => setName(e.target.value)}
                autoFocus
              />
            </div>
            <div className="field">
              <span className="label">Action</span>
              <select
                className="select"
                value={action}
                onChange={(e) => setAction(e.target.value as ScheduleAction)}
              >
                {Object.entries(ACTION_LABELS)
                  .filter(([id]) => !lockServer || id.startsWith("mc."))
                  .map(([id, label]) => (
                    <option key={id} value={id}>
                      {label}
                    </option>
                  ))}
              </select>
            </div>
          </div>

          {isMC && (
            <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: 12 }}>
              <div className="field">
                <span className="label">Server</span>
                {lockServer ? (
                  <span className="input mono" style={{ display: "flex", alignItems: "center" }}>
                    {lockServer}
                  </span>
                ) : (
                  <select className="select mono" value={server} onChange={(e) => setServer(e.target.value)}>
                    {mcServers.map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.name}
                      </option>
                    ))}
                  </select>
                )}
              </div>
              {action === "mc.backup" && (
                <div className="field">
                  <span className="label">Keep (old backups pruned; 0 = keep all)</span>
                  <input
                    className="input mono"
                    type="number"
                    min={0}
                    max={100}
                    value={keep}
                    onChange={(e) => setKeep(Number(e.target.value))}
                  />
                </div>
              )}
              {action === "mc.command" && (
                <div className="field">
                  <span className="label">Command</span>
                  <input
                    className="input mono"
                    placeholder="e.g. say Server restarts in 5 minutes"
                    value={command}
                    onChange={(e) => setCommand(e.target.value)}
                    spellCheck={false}
                  />
                </div>
              )}
            </div>
          )}

          {action === "service.restart" && (
            <div className="field">
              <span className="label">Unit</span>
              <select className="select mono" value={unit} onChange={(e) => setUnit(e.target.value)}>
                <option value="">choose…</option>
                {services
                  .filter((s) => s.loadState !== "not-found")
                  .map((s) => (
                    <option key={s.unit} value={s.unit}>
                      {s.unit}
                    </option>
                  ))}
              </select>
            </div>
          )}

          <div className="field">
            <span className="label">When</span>
            <div className="row wrap" style={{ gap: 8 }}>
              <div className="choice-row">
                {(["every", "daily", "weekly"] as WhenType[]).map((t) => (
                  <button
                    key={t}
                    className={`choice ${whenType === t ? "selected" : ""}`}
                    onClick={() => setWhenType(t)}
                  >
                    {t === "every" ? "interval" : t}
                  </button>
                ))}
              </div>
              {whenType === "every" && (
                <input
                  className="input mono"
                  style={{ width: 90 }}
                  value={every}
                  onChange={(e) => setEvery(e.target.value)}
                  placeholder="6h"
                  title="Go duration, e.g. 30m, 6h (min 5m)"
                />
              )}
              {whenType === "daily" && (
                <input
                  className="input mono"
                  type="time"
                  value={dailyTime}
                  onChange={(e) => setDailyTime(e.target.value)}
                />
              )}
              {whenType === "weekly" && (
                <>
                  <select
                    className="select mono"
                    value={weeklyDay}
                    onChange={(e) => setWeeklyDay(e.target.value)}
                  >
                    {WEEKDAYS.map((d) => (
                      <option key={d} value={d}>
                        {d}
                      </option>
                    ))}
                  </select>
                  <input
                    className="input mono"
                    type="time"
                    value={weeklyTime}
                    onChange={(e) => setWeeklyTime(e.target.value)}
                  />
                </>
              )}
            </div>
            {whenType === "every" && (
              <span className="hint">interval as a duration: 30m, 2h, 6h … (minimum 5m)</span>
            )}
          </div>

          <div className="row wrap" style={{ gap: 18 }}>
            {(action === "mc.backup" || action === "mc.command") && (
              <label className="row" style={{ gap: 8 }}>
                <Toggle checked={onlyIfRunning} onChange={setOnlyIfRunning} label="only if running" />
                <span className="small muted">only when the server is running</span>
              </label>
            )}
            <label className="row" style={{ gap: 8 }}>
              <Toggle checked={enabled} onChange={setEnabled} label="enabled" />
              <span className="small muted">enabled</span>
            </label>
          </div>

          <div className="small faint">
            Scheduled actions run unattended without per-action confirmation —
            they are limited to the panel's own allowlisted operations and every
            run is audited.
          </div>
        </div>
        <div className="modal-f">
          <button className="btn" onClick={() => onClose(false)}>
            Cancel
          </button>
          <button
            className="btn btn-primary"
            disabled={saving || !name.trim()}
            onClick={() => void save()}
          >
            {saving ? <Spinner size={13} /> : initial ? "Save" : "Create task"}
          </button>
        </div>
      </div>
    </div>
  );
}
