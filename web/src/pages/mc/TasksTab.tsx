// Per-server scheduled tasks: the same scheduler as the Tasks page, scoped
// to this Minecraft server — recurring console commands, restarts, and
// backups without leaving the server view.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { Schedule, SchedulesResponse } from "../../api/types";
import { EmptyState, Spinner } from "../../ui/bits";
import { Icon } from "../../ui/Icon";
import { ScheduleTable, TaskEditor } from "../Tasks";

export function TasksTab({ id }: { id: string }) {
  const qc = useQueryClient();
  const [editing, setEditing] = useState<Schedule | "new" | null>(null);
  const [preset, setPreset] = useState<Partial<Schedule> | undefined>();

  const { data, isLoading } = useQuery({
    queryKey: ["schedules"],
    queryFn: () => api<SchedulesResponse>("/api/schedules"),
    refetchInterval: 15_000,
  });
  const schedules = (data?.schedules ?? []).filter(
    (s) => s.action.startsWith("mc.") && s.server === id,
  );

  function openNew(p?: Partial<Schedule>) {
    setPreset(p);
    setEditing("new");
  }

  return (
    <div className="card-b" style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      <div className="row wrap">
        <span className="small muted">
          Recurring actions for this server — commands, restarts, backups. Runs
          are audited; job output shows in the usual places.
        </span>
        <div className="row right">
          <button
            className="btn btn-sm"
            onClick={() =>
              openNew({
                action: "mc.command",
                name: `${id}: scheduled command`,
                every: "6h",
                daily: undefined,
                onlyIfRunning: true,
              })
            }
          >
            <Icon name="terminal" size={12} />
            Scheduled command
          </button>
          <button className="btn btn-sm btn-primary" onClick={() => openNew()}>
            <Icon name="plus" size={12} />
            New task
          </button>
        </div>
      </div>

      {isLoading ? (
        <Spinner />
      ) : schedules.length === 0 ? (
        <EmptyState
          icon="clock"
          title="No tasks for this server"
          hint={
            <>
              Schedule console commands (announcements, save-all, whitelist
              changes), restarts, or backups. Example: a daily{" "}
              <span className="mono">say Server restarts in 5 minutes</span>{" "}
              followed by a restart task 5 minutes later.
            </>
          }
        />
      ) : (
        <ScheduleTable schedules={schedules} onEdit={(s) => setEditing(s)} />
      )}

      {editing && (
        <TaskEditor
          initial={editing === "new" ? null : editing}
          preset={editing === "new" ? preset : undefined}
          lockServer={id}
          onClose={(changed) => {
            setEditing(null);
            setPreset(undefined);
            if (changed) qc.invalidateQueries({ queryKey: ["schedules"] });
          }}
        />
      )}
    </div>
  );
}
