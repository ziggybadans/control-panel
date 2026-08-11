// Activity: the audit trail of every action taken through the panel.

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { AuditResponse } from "../api/types";
import { fmtDateTime } from "../lib/format";
import { Card, EmptyState, Spinner } from "../ui/bits";
import { Icon } from "../ui/Icon";

const PAGE_SIZE = 50;

export function ActivityPage() {
  const [page, setPage] = useState(0);
  const { data, isLoading } = useQuery({
    queryKey: ["audit", page],
    queryFn: () =>
      api<AuditResponse>(`/api/audit?offset=${page * PAGE_SIZE}&limit=${PAGE_SIZE}`),
    refetchInterval: page === 0 ? 10_000 : false,
  });

  const entries = data?.entries ?? [];
  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="page">
      <Card
        title={`Audit log · ${total} entries`}
        actions={
          pages > 1 ? (
            <div className="row small">
              <button className="btn btn-ghost btn-sm" disabled={page === 0} onClick={() => setPage(page - 1)}>
                ‹ Newer
              </button>
              <span className="muted num">
                {page + 1} / {pages}
              </span>
              <button
                className="btn btn-ghost btn-sm"
                disabled={page >= pages - 1}
                onClick={() => setPage(page + 1)}
              >
                Older ›
              </button>
            </div>
          ) : undefined
        }
        flush
      >
        {isLoading ? (
          <div className="empty">
            <Spinner size={18} />
          </div>
        ) : entries.length === 0 ? (
          <EmptyState
            icon="activity"
            title="No activity yet"
            hint="Every mutating action taken through the panel is recorded here."
          />
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th style={{ width: 160 }}>When</th>
                <th>Action</th>
                <th>Target</th>
                <th>Detail</th>
                <th style={{ width: 110 }}>Source</th>
                <th style={{ width: 70 }}>Result</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((e, i) => (
                <tr key={`${e.ts}-${i}`}>
                  <td className="num small muted">{fmtDateTime(e.ts)}</td>
                  <td className="mono small">{e.action}</td>
                  <td style={{ fontWeight: 550 }}>{e.target}</td>
                  <td className="small muted truncate" style={{ maxWidth: 260 }} title={e.err || e.detail}>
                    {e.err || e.detail || ""}
                  </td>
                  <td className="mono small muted">{e.ip}</td>
                  <td>
                    {e.ok ? (
                      <span className="badge ok">
                        <Icon name="check" size={11} />
                        ok
                      </span>
                    ) : (
                      <span className="badge crit">
                        <Icon name="x" size={11} />
                        failed
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </div>
  );
}
