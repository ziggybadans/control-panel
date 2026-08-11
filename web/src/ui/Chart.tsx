// Custom SVG time-series charts. Design rules (see dataviz method):
// one axis, thin 2px lines, hairline grid, subtle area fill, crosshair +
// tooltip on hover, legend for >=2 series, text in ink colors (never the
// series color for values).

import {
  useCallback,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  memo,
} from "react";
import { fmtTime } from "../lib/format";

export interface Series {
  name: string;
  /** CSS color (var(--chart-1) etc.). */
  color: string;
  points: (number | null)[];
}

interface Props {
  series: Series[];
  timestamps: number[];
  height?: number;
  yFormat: (v: number) => string;
  /** Fixed max (e.g. 100 for %). Otherwise auto with headroom. */
  yMax?: number;
  fill?: boolean;
}

const PAD_TOP = 6;
const PAD_BOTTOM = 4;
const PAD_RIGHT = 6;

function useWidth<T extends HTMLElement>(): [React.RefObject<T>, number] {
  const ref = useRef<T>(null);
  const [w, setW] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      setW(entries[0].contentRect.width);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  return [ref, w];
}

function niceMax(v: number): number {
  if (v <= 0) return 1;
  const exp = Math.floor(Math.log10(v));
  const base = 10 ** exp;
  for (const m of [1, 2, 2.5, 5, 10]) {
    if (m * base >= v) return m * base;
  }
  return 10 * base;
}

export const TimeSeriesChart = memo(function TimeSeriesChart({
  series,
  timestamps,
  height = 120,
  yFormat,
  yMax,
  fill = true,
}: Props) {
  const [wrapRef, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);

  const max = useMemo(() => {
    if (yMax !== undefined) return yMax;
    let m = 0;
    for (const s of series) {
      for (const v of s.points) {
        if (v !== null && v > m) m = v;
      }
    }
    return niceMax(m * 1.15);
  }, [series, yMax]);

  const n = timestamps.length;
  const plotW = Math.max(width - PAD_RIGHT, 0);
  const plotH = height - PAD_TOP - PAD_BOTTOM;
  const xAt = useCallback(
    (i: number) => (n <= 1 ? 0 : (i / (n - 1)) * plotW),
    [n, plotW],
  );
  const yAt = useCallback(
    (v: number) => PAD_TOP + plotH - (Math.min(v, max) / max) * plotH,
    [plotH, max],
  );

  const shapes = useMemo(() => {
    if (!width || n < 2) return [];
    return series.map((s) => {
      let line = "";
      let started = false;
      for (let i = 0; i < n; i++) {
        const v = s.points[i];
        if (v === null || v === undefined) {
          started = false;
          continue;
        }
        const cmd = started ? "L" : "M";
        line += `${cmd}${xAt(i).toFixed(1)},${yAt(v).toFixed(1)}`;
        started = true;
      }
      let area = "";
      if (fill && line) {
        area = `${line}L${xAt(n - 1).toFixed(1)},${(PAD_TOP + plotH).toFixed(1)}L${xAt(0).toFixed(1)},${(PAD_TOP + plotH).toFixed(1)}Z`;
      }
      return { line, area };
    });
  }, [series, width, n, xAt, yAt, fill, plotH]);

  const onMove = useCallback(
    (e: React.MouseEvent) => {
      if (!wrapRef.current || n < 2) return;
      const rect = wrapRef.current.getBoundingClientRect();
      const x = e.clientX - rect.left;
      const i = Math.round((x / Math.max(plotW, 1)) * (n - 1));
      setHover(Math.max(0, Math.min(n - 1, i)));
    },
    [n, plotW, wrapRef],
  );

  const gridYs = [0.25, 0.5, 0.75].map((f) => PAD_TOP + plotH * f);
  const hoverX = hover !== null ? xAt(hover) : 0;
  const tipLeft = hover !== null && hoverX > plotW * 0.6;

  return (
    <div
      ref={wrapRef}
      className="chart"
      style={{ height }}
      onMouseMove={onMove}
      onMouseLeave={() => setHover(null)}
    >
      {width > 0 && (
        <svg width={width} height={height} role="img">
          {gridYs.map((y) => (
            <line key={y} x1={0} x2={plotW} y1={y} y2={y} stroke="var(--grid)" strokeWidth={1} />
          ))}
          <line
            x1={0}
            x2={plotW}
            y1={PAD_TOP + plotH}
            y2={PAD_TOP + plotH}
            stroke="var(--border-strong)"
            strokeWidth={1}
          />
          {/* Y reference labels (max and mid) in muted ink. */}
          <text x={plotW} y={PAD_TOP + 3} textAnchor="end" className="chart-axis-label">
            {yFormat(max)}
          </text>
          <text x={plotW} y={PAD_TOP + plotH * 0.5 + 3} textAnchor="end" className="chart-axis-label">
            {yFormat(max / 2)}
          </text>
          {shapes.map((sh, i) =>
            sh.area ? (
              <path key={`a${i}`} d={sh.area} fill={series[i].color} opacity={0.1} stroke="none" />
            ) : null,
          )}
          {shapes.map((sh, i) => (
            <path
              key={`l${i}`}
              d={sh.line}
              fill="none"
              stroke={series[i].color}
              strokeWidth={1.75}
              strokeLinejoin="round"
            />
          ))}
          {hover !== null && (
            <>
              <line
                x1={hoverX}
                x2={hoverX}
                y1={PAD_TOP}
                y2={PAD_TOP + plotH}
                stroke="var(--border-strong)"
                strokeWidth={1}
              />
              {series.map((s, i) => {
                const v = s.points[hover];
                if (v === null || v === undefined) return null;
                return (
                  <circle
                    key={i}
                    cx={hoverX}
                    cy={yAt(v)}
                    r={3}
                    fill={s.color}
                    stroke="var(--surface)"
                    strokeWidth={2}
                  />
                );
              })}
            </>
          )}
        </svg>
      )}
      {hover !== null && timestamps[hover] !== undefined && (
        <div
          className="chart-tip"
          style={{
            top: 2,
            ...(tipLeft ? { right: width - hoverX + 8 } : { left: hoverX + 8 }),
          }}
        >
          <div className="faint">{fmtTime(timestamps[hover])}</div>
          {series.map((s) => {
            const v = s.points[hover];
            return (
              <div key={s.name} className="row" style={{ gap: 5 }}>
                <i
                  style={{
                    width: 7,
                    height: 7,
                    borderRadius: 2,
                    background: s.color,
                    display: "inline-block",
                  }}
                />
                <span className="muted">{s.name}</span>
                <span style={{ marginLeft: "auto", paddingLeft: 10 }}>
                  {v === null || v === undefined ? "—" : yFormat(v)}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
});

/** Tiny inline sparkline (no axes, no tooltip) for cards and table rows. */
export const Sparkline = memo(function Sparkline({
  points,
  color = "var(--chart-1)",
  width = 96,
  height = 24,
  max,
}: {
  points: (number | null)[];
  color?: string;
  width?: number;
  height?: number;
  max?: number;
}) {
  const path = useMemo(() => {
    const vals = points.filter((v): v is number => v !== null);
    if (vals.length < 2) return "";
    const m = max ?? Math.max(...vals, 1e-9) * 1.1;
    const n = points.length;
    let d = "";
    let started = false;
    for (let i = 0; i < n; i++) {
      const v = points[i];
      if (v === null) {
        started = false;
        continue;
      }
      const x = (i / (n - 1)) * (width - 2) + 1;
      const y = height - 2 - (Math.min(v, m) / m) * (height - 4);
      d += `${started ? "L" : "M"}${x.toFixed(1)},${y.toFixed(1)}`;
      started = true;
    }
    return d;
  }, [points, width, height, max]);

  return (
    <svg width={width} height={height} aria-hidden="true" style={{ flex: "none" }}>
      {path && (
        <path d={path} fill="none" stroke={color} strokeWidth={1.5} strokeLinejoin="round" />
      )}
    </svg>
  );
});
