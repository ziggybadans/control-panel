// Live metric chart widgets (CPU, memory, network, disk I/O).

import { useMemo } from "react";
import type { Snapshot } from "../../api/types";
import { fmtBytes, fmtPct, fmtRate } from "../../lib/format";
import { useLatestMetrics, useMetricsHistory } from "../../state/live";
import { TimeSeriesChart } from "../../ui/Chart";

function useHistorySeries(
  selectors: ((s: Snapshot) => number | null)[],
): { timestamps: number[]; points: (number | null)[][] } {
  const { history, rev } = useMetricsHistory();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  return useMemo(
    () => ({
      timestamps: history.map((h) => h.ts),
      points: selectors.map((sel) => history.map(sel)),
    }),
    // rev is the ring's change signal; selectors are stable per widget.
    [rev],
  );
}

// --- CPU --------------------------------------------------------------------

export function CpuHeader() {
  const m = useLatestMetrics();
  return (
    <div className="widget-head-stats">
      <span className="small muted num">
        load {m ? m.load.map((l) => l.toFixed(2)).join(" · ") : "—"}
      </span>
      <span style={{ fontWeight: 650, fontSize: "var(--fs-lg)" }}>
        {m ? fmtPct(m.cpu) : "—"}
      </span>
    </div>
  );
}

export function CpuWidget() {
  const m = useLatestMetrics();
  const { timestamps, points } = useHistorySeries([(s) => s.cpu]);
  return (
    <div>
      <TimeSeriesChart
        series={[{ name: "CPU", color: "var(--chart-1)", points: points[0] }]}
        timestamps={timestamps}
        yFormat={(v) => fmtPct(v)}
        yMax={100}
        height={110}
      />
      <div className="core-bars" title="per-core utilisation">
        {(m?.perCore ?? []).map((c, i) => (
          <i key={i} style={{ height: `${Math.max(c, 3)}%` }} />
        ))}
      </div>
    </div>
  );
}

// --- Memory -----------------------------------------------------------------

export function MemoryHeader() {
  const m = useLatestMetrics();
  return (
    <div className="widget-head-stats">
      <span className="small muted num">
        {m ? `${fmtBytes(m.memUsed)} / ${fmtBytes(m.memTotal)}` : ""}
      </span>
      <span style={{ fontWeight: 650, fontSize: "var(--fs-lg)" }}>
        {m && m.memTotal ? fmtPct((m.memUsed / m.memTotal) * 100) : "—"}
      </span>
    </div>
  );
}

export function MemoryWidget() {
  const m = useLatestMetrics();
  const { timestamps, points } = useHistorySeries([
    (s) => s.memUsed,
    (s) => s.memCached,
  ]);
  const total = m?.memTotal ?? 0;
  return (
    <div>
      <TimeSeriesChart
        series={[
          { name: "Used", color: "var(--chart-1)", points: points[0] },
          { name: "Cached", color: "var(--chart-2)", points: points[1] },
        ]}
        timestamps={timestamps}
        yFormat={(v) => fmtBytes(v, 0)}
        yMax={total || undefined}
        height={110}
      />
      <div className="row" style={{ marginTop: 8 }}>
        <div className="legend">
          <span>
            <i style={{ background: "var(--chart-1)" }} />
            Used
          </span>
          <span>
            <i style={{ background: "var(--chart-2)" }} />
            Cached
          </span>
        </div>
        <span className="small muted right num">
          swap {m ? `${fmtBytes(m.swapUsed, 0)} / ${fmtBytes(m.swapTotal, 0)}` : "—"}
        </span>
      </div>
    </div>
  );
}

// --- Network ----------------------------------------------------------------

const sumRx = (s: Snapshot) => s.net?.reduce((a, n) => a + n.rxBps, 0) ?? 0;
const sumTx = (s: Snapshot) => s.net?.reduce((a, n) => a + n.txBps, 0) ?? 0;

export function NetworkHeader() {
  const m = useLatestMetrics();
  return (
    <div className="widget-head-stats small num">
      <span className="muted">
        ↓ <span style={{ color: "var(--text)" }}>{m ? fmtRate(sumRx(m)) : "—"}</span>
      </span>
      <span className="muted">
        ↑ <span style={{ color: "var(--text)" }}>{m ? fmtRate(sumTx(m)) : "—"}</span>
      </span>
    </div>
  );
}

export function NetworkWidget() {
  const m = useLatestMetrics();
  const { timestamps, points } = useHistorySeries([sumRx, sumTx]);
  return (
    <div>
      <TimeSeriesChart
        series={[
          { name: "Down", color: "var(--chart-1)", points: points[0] },
          { name: "Up", color: "var(--chart-2)", points: points[1] },
        ]}
        timestamps={timestamps}
        yFormat={fmtRate}
        height={110}
      />
      <div className="row" style={{ marginTop: 8 }}>
        <div className="legend">
          <span>
            <i style={{ background: "var(--chart-1)" }} />
            Down
          </span>
          <span>
            <i style={{ background: "var(--chart-2)" }} />
            Up
          </span>
        </div>
        <span className="small muted right">
          {(m?.net ?? []).map((n) => n.name).join(", ") || "no interfaces"}
        </span>
      </div>
    </div>
  );
}

// --- Disk I/O ---------------------------------------------------------------

const sumRead = (s: Snapshot) => s.disk?.reduce((a, d) => a + d.readBps, 0) ?? 0;
const sumWrite = (s: Snapshot) => s.disk?.reduce((a, d) => a + d.writeBps, 0) ?? 0;

export function DiskIOHeader() {
  const m = useLatestMetrics();
  return (
    <div className="widget-head-stats small num">
      <span className="muted">
        R <span style={{ color: "var(--text)" }}>{m ? fmtRate(sumRead(m)) : "—"}</span>
      </span>
      <span className="muted">
        W <span style={{ color: "var(--text)" }}>{m ? fmtRate(sumWrite(m)) : "—"}</span>
      </span>
    </div>
  );
}

export function DiskIOWidget() {
  const m = useLatestMetrics();
  const { timestamps, points } = useHistorySeries([sumRead, sumWrite]);
  const busiest = useMemo(() => {
    if (!m?.disk?.length) return null;
    return [...m.disk].sort((a, b) => b.utilPct - a.utilPct)[0];
  }, [m]);
  return (
    <div>
      <TimeSeriesChart
        series={[
          { name: "Read", color: "var(--chart-1)", points: points[0] },
          { name: "Write", color: "var(--chart-2)", points: points[1] },
        ]}
        timestamps={timestamps}
        yFormat={fmtRate}
        height={110}
      />
      <div className="row" style={{ marginTop: 8 }}>
        <div className="legend">
          <span>
            <i style={{ background: "var(--chart-1)" }} />
            Read
          </span>
          <span>
            <i style={{ background: "var(--chart-2)" }} />
            Write
          </span>
        </div>
        {busiest && (
          <span className="small muted right num">
            busiest {busiest.name} · {fmtPct(busiest.utilPct)}
          </span>
        )}
      </div>
    </div>
  );
}
