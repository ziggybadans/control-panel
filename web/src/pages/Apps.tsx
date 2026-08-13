// Media apps page: one card per configured app (Radarr, Sonarr, Prowlarr,
// Overseerr/Jellyseerr…) with health, queue progress, and library stats.
// Read-only by design: the panel holds the API keys server-side and links
// out to each app's own UI for actions.

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { AppStatus, AppsResponse } from "../api/types";
import { fmtPct } from "../lib/format";
import { externalURL } from "../lib/url";
import { Card, EmptyState, Spinner } from "../ui/bits";
import { Icon } from "../ui/Icon";

export function AppsPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["apps"],
    queryFn: () => api<AppsResponse>("/api/apps"),
    refetchInterval: 10_000,
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

  const apps = data?.apps ?? [];

  if (!data?.configured || apps.length === 0) {
    return (
      <div className="page">
        <Card title="Media apps">
          <EmptyState
            icon="download"
            title="No media apps configured"
            hint={
              <>
                List your Radarr / Sonarr / Prowlarr / Jellyseerr instances under{" "}
                <span className="mono">apps:</span> in config.yaml with their URLs and
                API keys. The panel shows queues, health, and requests — read-only.
              </>
            }
          />
        </Card>
      </div>
    );
  }

  return (
    <div className="page">
      <div
        className="grid"
        style={{ gridTemplateColumns: "repeat(auto-fill, minmax(360px, 1fr))" }}
      >
        {apps.map((app) => (
          <AppCard key={`${app.type}-${app.name}`} app={app} />
        ))}
      </div>
    </div>
  );
}

function AppCard({ app }: { app: AppStatus }) {
  const issues = app.healthIssues ?? [];
  return (
    <div className="card">
      <div className="card-h">
        <span
          className={`dot ${app.reachable ? (issues.length > 0 ? "warn" : "ok") : "crit"}`}
        />
        <span style={{ fontWeight: 650 }}>{app.name}</span>
        <span className="small faint">{app.version}</span>
        <div className="row right">
          {app.reachable && issues.length > 0 && (
            <span className="badge warn">
              {issues.length} issue{issues.length > 1 ? "s" : ""}
            </span>
          )}
          <a
            className="btn btn-ghost btn-sm"
            href={externalURL(app.url)}
            target="_blank"
            rel="noreferrer"
            title={`Open ${app.name}`}
          >
            <Icon name="external" size={12} />
            Open
          </a>
        </div>
      </div>
      <div className="card-b" style={{ display: "flex", flexDirection: "column", gap: 10 }}>
        {!app.reachable ? (
          <div className="small crit-text">{app.error || "unreachable"}</div>
        ) : (
          <>
            {issues.map((issue, i) => (
              <div key={i} className="row small warn-text" style={{ alignItems: "flex-start" }}>
                <Icon name="warning" size={12} />
                <span>{issue}</span>
              </div>
            ))}

            {app.requests ? (
              <RequestStats app={app} />
            ) : (
              <ArrStats app={app} />
            )}
          </>
        )}
      </div>
    </div>
  );
}

function ArrStats({ app }: { app: AppStatus }) {
  const queue = app.queue ?? [];
  const isIndexer = app.type === "prowlarr";
  return (
    <>
      {!isIndexer && (
        <div className="row small muted num" style={{ gap: 14 }}>
          <span>
            queue <b style={{ color: "var(--text)" }}>{app.queueCount}</b>
          </span>
          <span>
            missing <b style={{ color: "var(--text)" }}>{app.missing ?? 0}</b>
          </span>
          <span>
            next 7d <b style={{ color: "var(--text)" }}>{app.upcomingWeek ?? 0}</b>
          </span>
        </div>
      )}
      {queue.length > 0 && (
        <div className="mini-rows">
          {queue.map((item, i) => (
            <div key={i} className="mini-row" style={{ alignItems: "stretch", flexDirection: "column", gap: 3 }}>
              <div className="row small">
                <span className="truncate" style={{ fontWeight: 550 }}>
                  {item.title}
                </span>
                <span className="right faint num">
                  {item.timeLeft ? `${item.timeLeft} · ` : ""}
                  {fmtPct(item.progress)}
                </span>
              </div>
              <div className="bar">
                <i style={{ width: `${Math.min(item.progress, 100).toFixed(1)}%` }} />
              </div>
            </div>
          ))}
        </div>
      )}
      {isIndexer && queue.length === 0 && (
        <div className="small faint">Indexer manager — healthy when quiet.</div>
      )}
      {!isIndexer && queue.length === 0 && (
        <div className="small faint">Queue is empty.</div>
      )}
    </>
  );
}

function RequestStats({ app }: { app: AppStatus }) {
  const r = app.requests!;
  return (
    <div className="row small num wrap" style={{ gap: 14 }}>
      <span className={r.pending > 0 ? "warn-text" : "muted"}>
        pending <b>{r.pending}</b>
      </span>
      <span className="muted">
        processing <b style={{ color: "var(--text)" }}>{r.processing}</b>
      </span>
      <span className="muted">
        approved <b style={{ color: "var(--text)" }}>{r.approved}</b>
      </span>
      <span className="muted">
        available <b style={{ color: "var(--text)" }}>{r.available}</b>
      </span>
    </div>
  );
}
