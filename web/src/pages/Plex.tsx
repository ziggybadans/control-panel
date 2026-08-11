// Plex page: server status, active sessions, library statistics, and
// service control (through the systemd unit).

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { PlexStatus } from "../api/types";
import { fmtDuration } from "../lib/format";
import { useServices } from "../state/live";
import { Card, EmptyState, ServiceBadge, Spinner } from "../ui/bits";
import { Icon } from "../ui/Icon";

export function PlexPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["plex"],
    queryFn: () => api<PlexStatus>("/api/plex"),
    refetchInterval: 8_000,
  });
  const services = useServices();
  const plexService = services.find((s) => s.unit === "plexmediaserver.service");

  if (isLoading || !data) {
    return (
      <div className="page">
        <div className="empty">
          <Spinner size={20} />
        </div>
      </div>
    );
  }

  if (!data.configured) {
    return (
      <div className="page">
        <Card title="Plex">
          <EmptyState
            icon="plex"
            title="Plex integration is not configured"
            hint={
              <>
                Add your server token as <span className="mono">plex.token</span> in
                config.yaml (find it via any Plex Web request's{" "}
                <span className="mono">X-Plex-Token</span> parameter).
              </>
            }
          />
        </Card>
      </div>
    );
  }

  return (
    <div className="page">
      <div className="grid" style={{ gridTemplateColumns: "2fr 1fr" }}>
        <Card title={`Now playing · ${data.sessions.length}`}>
          {!data.reachable ? (
            <div className="crit-text small">Unreachable: {data.error}</div>
          ) : data.sessions.length === 0 ? (
            <EmptyState icon="plex" title="Nothing is playing" />
          ) : (
            <div className="mini-rows">
              {data.sessions.map((s, i) => (
                <div key={i} className="mini-row" style={{ padding: "10px 0" }}>
                  <div className="grow" style={{ minWidth: 0 }}>
                    <div className="row">
                      <span style={{ fontWeight: 600 }} className="truncate">
                        {s.grandparent ? `${s.grandparent} — ${s.title}` : s.title}
                      </span>
                      {s.decision === "transcode" ? (
                        <span className="badge warn">transcode</span>
                      ) : (
                        <span className="badge ok">direct play</span>
                      )}
                      {s.state === "paused" && <span className="badge neutral">paused</span>}
                    </div>
                    <div className="small muted" style={{ marginTop: 2 }}>
                      {s.user} · {s.player} ({s.product})
                      {s.bitrateKbps ? ` · ${(s.bitrateKbps / 1000).toFixed(1)} Mbps` : ""}
                    </div>
                    <div className="row" style={{ marginTop: 6, gap: 10 }}>
                      <div className="bar grow">
                        <i
                          style={{
                            width: `${s.durationMs ? ((s.progressMs / s.durationMs) * 100).toFixed(1) : 0}%`,
                          }}
                        />
                      </div>
                      <span className="small faint num">
                        {fmtDuration(s.progressMs)} / {fmtDuration(s.durationMs)}
                      </span>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>

        <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
          <Card title="Server">
            <dl className="kv">
              <dt>Status</dt>
              <dd>
                {plexService ? (
                  <ServiceBadge
                    active={plexService.activeState}
                    sub={plexService.subState}
                  />
                ) : data.reachable ? (
                  <span className="badge ok">reachable</span>
                ) : (
                  <span className="badge crit">unreachable</span>
                )}
              </dd>
              <dt>Version</dt>
              <dd className="num">{data.version || "—"}</dd>
              <dt>Streams</dt>
              <dd className="num">{data.sessions.length}</dd>
              <dt>Transcodes</dt>
              <dd className="num">
                {data.sessions.filter((s) => s.decision === "transcode").length}
              </dd>
            </dl>
            <div className="row" style={{ marginTop: 10 }}>
              <a className="btn btn-sm" href="http://app.plex.tv" target="_blank" rel="noreferrer">
                <Icon name="external" size={12} />
                Open Plex Web
              </a>
              <span className="small faint right">manage the unit in Services</span>
            </div>
          </Card>

          <Card title="Libraries" flush>
            <table className="table">
              <thead>
                <tr>
                  <th>Library</th>
                  <th>Type</th>
                  <th className="r">Items</th>
                </tr>
              </thead>
              <tbody>
                {data.libraries.map((l) => (
                  <tr key={l.title}>
                    <td style={{ fontWeight: 550 }}>{l.title}</td>
                    <td className="small muted">{l.type}</td>
                    <td className="r num">{l.count.toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Card>
        </div>
      </div>
    </div>
  );
}
