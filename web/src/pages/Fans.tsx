// Fan control: per-fan mode (auto / manual / curve), an interactive
// temp→duty curve editor, and live RPM/duty from the SSE stream. Applying
// settings confirms server-side (X-Confirm on the fan id).

import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type {
  FanCurvePoint,
  FanSensor,
  FanSettings,
  FanState,
  FansSnapshot,
} from "../api/types";
import { fmtTemp } from "../lib/format";
import { useFansLive } from "../state/live";
import { EmptyState, Spinner } from "../ui/bits";
import { useConfirm } from "../ui/Confirm";
import { Icon } from "../ui/Icon";
import { useToast } from "../ui/Toast";

const DEFAULT_CURVE: FanCurvePoint[] = [
  { tempC: 35, dutyPct: 20 },
  { tempC: 50, dutyPct: 35 },
  { tempC: 65, dutyPct: 65 },
  { tempC: 78, dutyPct: 100 },
];

export function FansPage() {
  const { data } = useQuery({
    queryKey: ["fans"],
    queryFn: () => api<FansSnapshot>("/api/fans"),
    staleTime: 10_000,
  });
  const live = useFansLive();

  if (!data) {
    return (
      <div className="page">
        <Spinner />
      </div>
    );
  }
  if (!data.supported) {
    return (
      <div className="page">
        <EmptyState
          icon="fan"
          title="No controllable fans detected"
          hint={
            <>
              No hwmon PWM outputs were found. On most boards the Super-I/O
              sensor driver provides them — try{" "}
              <span className="mono">modprobe nct6775</span> (or the driver for
              your chip) and restart the panel.
            </>
          }
        />
      </div>
    );
  }

  const fans = data.fans ?? [];
  const liveFans = live.fans ?? [];
  const sensors = (live.sensors?.length ? live.sensors : data.sensors) ?? [];
  const current = (f: FanState) => liveFans.find((x) => x.id === f.id) ?? f;
  // Headers reporting 0 rpm while untouched are almost certainly empty;
  // fold them away (they stay fully controllable inside the fold).
  const unused = fans.filter((f) => {
    const c = current(f);
    return c.rpm === 0 && c.mode === "auto" && !c.err;
  });
  const active = fans.filter((f) => !unused.includes(f));

  return (
    <div className="page">
      {!data.control && (
        <div className="row small">
          <span className="badge warn">monitoring only</span>
          <span className="muted">
            Fan control is disabled — set <span className="mono">fans.control: true</span>{" "}
            in config.yaml to apply curves.
          </span>
        </div>
      )}
      <div className="fans-grid">
        {active.map((f) => (
          <FanCard
            key={f.id}
            fan={current(f)}
            saved={data.settings?.[f.id]}
            sensors={sensors}
            control={data.control}
          />
        ))}
      </div>
      {unused.length > 0 && (
        <details className="fans-unused">
          <summary className="small muted">
            {unused.length} unconnected header{unused.length > 1 ? "s" : ""} (0 rpm,
            firmware control)
          </summary>
          <div className="fans-grid" style={{ marginTop: 10 }}>
            {unused.map((f) => (
              <FanCard
                key={f.id}
                fan={current(f)}
                saved={data.settings?.[f.id]}
                sensors={sensors}
                control={data.control}
              />
            ))}
          </div>
        </details>
      )}
      <div className="small faint">
        Fans stay under firmware control until a manual duty or curve is applied.
        If a curve's sensor becomes unreadable, the fan is driven to 100% as a
        failsafe. The panel hands fans back to firmware when it shuts down.
      </div>
    </div>
  );
}

function settingsEqual(a: FanSettings, b: FanSettings): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

/** Normalized draft from saved settings (or sensible defaults). */
function draftFrom(saved: FanSettings | undefined, sensors: FanSensor[]): FanSettings {
  return {
    mode: saved?.mode ?? "auto",
    manualPct: saved?.manualPct ?? 50,
    sensor: saved?.sensor ?? sensors[0]?.id,
    points: saved?.points?.length ? saved.points : DEFAULT_CURVE,
  };
}

function FanCard({
  fan,
  saved,
  sensors,
  control,
}: {
  fan: FanState;
  saved: FanSettings | undefined;
  sensors: FanSensor[];
  control: boolean;
}) {
  const confirm = useConfirm();
  const toast = useToast();
  const qc = useQueryClient();
  const [draft, setDraft] = useState<FanSettings>(() => draftFrom(saved, sensors));
  const [busy, setBusy] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const [nameValue, setNameValue] = useState("");
  // Re-sync the draft when the server copy changes (apply elsewhere/reload).
  const savedKey = JSON.stringify(saved ?? null);
  useEffect(() => {
    setDraft(draftFrom(saved, sensors));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [savedKey]);

  const dirty = !settingsEqual(
    normalize(draft),
    normalize(draftFrom(saved, sensors)),
  );

  const liveSensor = sensors.find((s) => s.id === draft.sensor);

  async function apply() {
    const body = normalize(draft);
    const ok = await confirm({
      title: `Apply fan settings — ${fan.label}`,
      target: fan.id,
      body:
        body.mode === "auto" ? (
          <>
            <b>{fan.label}</b> returns to firmware (BIOS) control.
          </>
        ) : body.mode === "manual" ? (
          <>
            <b>{fan.label}</b> will run at a fixed <b>{body.manualPct}%</b> duty
            until changed. Make sure this keeps temperatures in check under
            load.
          </>
        ) : (
          <>
            <b>{fan.label}</b> will follow the curve on{" "}
            <b>{liveSensor?.label ?? body.sensor}</b>. If that sensor becomes
            unreadable the fan runs at 100% until it recovers.
          </>
        ),
      confirmLabel: "Apply",
    });
    if (!ok) return;
    setBusy(true);
    try {
      await api(`/api/fans/${encodeURIComponent(fan.id)}`, {
        method: "PUT",
        body,
        confirm: fan.id,
      });
      toast("ok", `${fan.label}: ${body.mode} applied`);
      await qc.invalidateQueries({ queryKey: ["fans"] });
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "apply failed");
    } finally {
      setBusy(false);
    }
  }

  async function saveName() {
    setRenaming(false);
    try {
      await api(`/api/fans/${encodeURIComponent(fan.id)}/name`, {
        method: "PUT",
        body: { name: nameValue.trim() },
      });
      toast("ok", nameValue.trim() ? `renamed to ${nameValue.trim()}` : "name cleared");
      await qc.invalidateQueries({ queryKey: ["fans"] });
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "rename failed");
    }
  }

  return (
    <div className="card">
      <div className="card-h">
        <Icon name="fan" size={14} className="faint" />
        {renaming ? (
          <input
            className="input"
            style={{ height: 22, fontSize: 12, maxWidth: 180 }}
            value={nameValue}
            placeholder={fan.hwLabel ?? fan.label}
            autoFocus
            maxLength={40}
            onChange={(e) => setNameValue(e.target.value)}
            onBlur={() => void saveName()}
            onKeyDown={(e) => {
              if (e.key === "Enter") void saveName();
              if (e.key === "Escape") setRenaming(false);
            }}
          />
        ) : (
          <>
            <span
              className="label"
              title={fan.hwLabel && fan.hwLabel !== fan.label ? fan.hwLabel : undefined}
            >
              {fan.label}
            </span>
            <button
              className="btn btn-ghost btn-sm btn-icon"
              title="Rename fan (empty restores the hardware label)"
              onClick={() => {
                setNameValue(fan.hwLabel !== fan.label ? fan.label : "");
                setRenaming(true);
              }}
            >
              <Icon name="edit" size={11} />
            </button>
          </>
        )}
        <div className="row right small">
          {fan.failsafe && <span className="badge crit">failsafe 100%</span>}
          {!fan.writable && (
            <span
              className="badge neutral"
              title="The kernel driver exposes this fan's PWM read-only — monitoring works, control does not."
            >
              read-only
            </span>
          )}
          {fan.err && !fan.failsafe && fan.writable && (
            <span className="badge crit" title={fan.err}>
              error
            </span>
          )}
          <span className="num muted">
            {fan.rpm >= 0 ? `${fan.rpm} rpm` : "no tach"}
          </span>
          <span className="faint">·</span>
          <span className="num muted">{Math.round(fan.dutyPct)}%</span>
          <span
            className={
              fan.mode === "auto" ? "badge neutral" : fan.mode === "manual" ? "badge warn" : "badge ok"
            }
          >
            {fan.mode}
          </span>
        </div>
      </div>
      <div className="card-b" style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        <div className="choice-row" role="group" aria-label="fan mode">
          {(["auto", "manual", "curve"] as const).map((m) => (
            <button
              key={m}
              className={`choice ${draft.mode === m ? "selected" : ""}`}
              onClick={() => setDraft({ ...draft, mode: m })}
              disabled={(!control || !fan.writable) && m !== "auto"}
            >
              {m}
            </button>
          ))}
        </div>

        {!fan.writable && (
          <div className="small muted">
            The kernel driver exposes this fan's PWM read-only, so only
            monitoring is possible.
            {(fan.hwLabel ?? fan.label).startsWith("nct6687") && (
              <>
                {" "}
                On NCT6687D boards the out-of-tree{" "}
                <a
                  href="https://github.com/Fred78290/nct6687d"
                  target="_blank"
                  rel="noreferrer"
                >
                  nct6687d driver
                </a>{" "}
                enables full control.
              </>
            )}
          </div>
        )}

        {draft.mode === "manual" && (
          <div className="row" style={{ gap: 12 }}>
            <input
              type="range"
              className="slider"
              min={0}
              max={100}
              step={1}
              value={draft.manualPct ?? 50}
              onChange={(e) => setDraft({ ...draft, manualPct: Number(e.target.value) })}
              style={{ flex: 1 }}
              aria-label="manual duty"
            />
            <span className="num" style={{ minWidth: 42, textAlign: "right" }}>
              {draft.manualPct ?? 50}%
            </span>
          </div>
        )}

        {draft.mode === "curve" && (
          <>
            <div className="row small">
              <span className="muted">Sensor</span>
              <select
                className="input"
                style={{ flex: 1, maxWidth: 300 }}
                value={draft.sensor ?? ""}
                onChange={(e) => setDraft({ ...draft, sensor: e.target.value })}
              >
                {sensors.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.label} — {fmtTemp(s.c)}
                  </option>
                ))}
              </select>
            </div>
            <CurveEditor
              points={draft.points ?? DEFAULT_CURVE}
              onChange={(points) => setDraft({ ...draft, points })}
              liveTempC={liveSensor?.c}
            />
            <div className="small faint">
              Drag points · double-click to add · right-click a point to remove.
            </div>
          </>
        )}

        <div className="row">
          <button
            className="btn btn-sm btn-primary"
            disabled={
              !dirty || busy || ((!control || !fan.writable) && draft.mode !== "auto")
            }
            onClick={() => void apply()}
          >
            {busy ? <Spinner size={11} /> : <Icon name="check" size={12} />}
            Apply
          </button>
          {dirty && (
            <button
              className="btn btn-sm btn-ghost"
              disabled={busy}
              onClick={() => setDraft(draftFrom(saved, sensors))}
            >
              Revert
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

/** Strips fields irrelevant to the selected mode so drafts compare cleanly. */
function normalize(s: FanSettings): FanSettings {
  switch (s.mode) {
    case "manual":
      return { mode: "manual", manualPct: s.manualPct ?? 50 };
    case "curve":
      return { mode: "curve", sensor: s.sensor, points: s.points };
    default:
      return { mode: "auto" };
  }
}

// --- curve editor ------------------------------------------------------------

const X_MIN = 0;
const X_MAX = 100; // °C
const PAD = { top: 10, right: 10, bottom: 20, left: 34 };
const EDITOR_H = 190;
const MAX_POINTS = 12;

function CurveEditor({
  points,
  onChange,
  liveTempC,
}: {
  points: FanCurvePoint[];
  onChange: (p: FanCurvePoint[]) => void;
  liveTempC?: number;
}) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [dragIdx, setDragIdx] = useState<number | null>(null);

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const ro = new ResizeObserver((es) => setWidth(es[0].contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const plotW = Math.max(width - PAD.left - PAD.right, 0);
  const plotH = EDITOR_H - PAD.top - PAD.bottom;
  const xAt = (t: number) => PAD.left + ((t - X_MIN) / (X_MAX - X_MIN)) * plotW;
  const yAt = (d: number) => PAD.top + plotH - (d / 100) * plotH;
  const tempAt = (x: number) =>
    Math.round(Math.min(X_MAX, Math.max(X_MIN, ((x - PAD.left) / Math.max(plotW, 1)) * (X_MAX - X_MIN))));
  const dutyAt = (y: number) =>
    Math.round(Math.min(100, Math.max(0, ((PAD.top + plotH - y) / Math.max(plotH, 1)) * 100)));

  // The applied curve is flat outside the outermost points.
  const path = useMemo(() => {
    if (!points.length || !plotW) return "";
    let d = `M${PAD.left},${yAt(points[0].dutyPct).toFixed(1)}`;
    for (const p of points) d += `L${xAt(p.tempC).toFixed(1)},${yAt(p.dutyPct).toFixed(1)}`;
    d += `L${(PAD.left + plotW).toFixed(1)},${yAt(points[points.length - 1].dutyPct).toFixed(1)}`;
    return d;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [points, plotW, plotH]);

  function svgPoint(e: React.PointerEvent | React.MouseEvent): { x: number; y: number } {
    const rect = (wrapRef.current as HTMLDivElement).getBoundingClientRect();
    return { x: e.clientX - rect.left, y: e.clientY - rect.top };
  }

  function moveTo(idx: number, e: React.PointerEvent) {
    const { x, y } = svgPoint(e);
    const next = points.map((p) => ({ ...p }));
    const lo = idx > 0 ? points[idx - 1].tempC + 1 : X_MIN;
    const hi = idx < points.length - 1 ? points[idx + 1].tempC - 1 : X_MAX;
    next[idx].tempC = Math.min(hi, Math.max(lo, tempAt(x)));
    next[idx].dutyPct = dutyAt(y);
    onChange(next);
  }

  function addPoint(e: React.MouseEvent) {
    if (points.length >= MAX_POINTS) return;
    const { x, y } = svgPoint(e);
    const t = tempAt(x);
    if (points.some((p) => Math.abs(p.tempC - t) < 1)) return;
    const next = [...points, { tempC: t, dutyPct: dutyAt(y) }].sort(
      (a, b) => a.tempC - b.tempC,
    );
    onChange(next);
  }

  function removePoint(idx: number, e: React.MouseEvent) {
    e.preventDefault();
    if (points.length <= 2) return;
    onChange(points.filter((_, i) => i !== idx));
  }

  // Live marker: where the applied curve would put this fan right now.
  const liveDuty = useMemo(() => {
    if (liveTempC === undefined || !points.length) return undefined;
    const sorted = points;
    if (liveTempC <= sorted[0].tempC) return sorted[0].dutyPct;
    const last = sorted[sorted.length - 1];
    if (liveTempC >= last.tempC) return last.dutyPct;
    const i = sorted.findIndex((p) => p.tempC >= liveTempC);
    const a = sorted[i - 1];
    const b = sorted[i];
    return a.dutyPct + ((liveTempC - a.tempC) / (b.tempC - a.tempC)) * (b.dutyPct - a.dutyPct);
  }, [liveTempC, points]);

  return (
    <div ref={wrapRef} className="curve-editor" style={{ height: EDITOR_H }}>
      {width > 0 && (
        <svg
          width={width}
          height={EDITOR_H}
          role="application"
          aria-label="fan curve editor"
          onDoubleClick={addPoint}
        >
          {/* Grid + axis labels (recessive, ink-colored text). */}
          {[0, 25, 50, 75, 100].map((d) => (
            <g key={d}>
              <line
                x1={PAD.left}
                x2={PAD.left + plotW}
                y1={yAt(d)}
                y2={yAt(d)}
                stroke={d === 0 ? "var(--border-strong)" : "var(--grid)"}
                strokeWidth={1}
              />
              <text x={PAD.left - 6} y={yAt(d) + 3} textAnchor="end" className="chart-axis-label">
                {d}%
              </text>
            </g>
          ))}
          {[0, 20, 40, 60, 80, 100].map((t) => (
            <text
              key={t}
              x={xAt(t)}
              y={EDITOR_H - 5}
              textAnchor="middle"
              className="chart-axis-label"
            >
              {t}°
            </text>
          ))}

          <path d={`${path}L${PAD.left + plotW},${PAD.top + plotH}L${PAD.left},${PAD.top + plotH}Z`} fill="var(--chart-1)" opacity={0.08} stroke="none" />
          <path d={path} fill="none" stroke="var(--chart-1)" strokeWidth={2} strokeLinejoin="round" />

          {/* Live operating point on the draft curve. */}
          {liveTempC !== undefined && liveDuty !== undefined && (
            <g>
              <line
                x1={xAt(liveTempC)}
                x2={xAt(liveTempC)}
                y1={PAD.top}
                y2={PAD.top + plotH}
                stroke="var(--border-strong)"
                strokeDasharray="3 3"
                strokeWidth={1}
              />
              <circle cx={xAt(liveTempC)} cy={yAt(liveDuty)} r={4} fill="var(--chart-2)" stroke="var(--surface)" strokeWidth={2} />
              <text
                x={Math.min(xAt(liveTempC) + 8, PAD.left + plotW - 4)}
                y={Math.max(yAt(liveDuty) - 8, PAD.top + 10)}
                textAnchor={xAt(liveTempC) > PAD.left + plotW * 0.7 ? "end" : "start"}
                className="chart-axis-label"
              >
                {`${liveTempC.toFixed(0)}° → ${Math.round(liveDuty)}%`}
              </text>
            </g>
          )}

          {/* Draggable points (large invisible hit targets). */}
          {points.map((p, i) => (
            <g key={i}>
              <circle
                cx={xAt(p.tempC)}
                cy={yAt(p.dutyPct)}
                r={12}
                fill="transparent"
                className="curve-pt-hit"
                onPointerDown={(e) => {
                  (e.target as Element).setPointerCapture(e.pointerId);
                  setDragIdx(i);
                }}
                onPointerMove={(e) => {
                  if (dragIdx === i) moveTo(i, e);
                }}
                onPointerUp={() => setDragIdx(null)}
                onContextMenu={(e) => removePoint(i, e)}
              />
              <circle
                cx={xAt(p.tempC)}
                cy={yAt(p.dutyPct)}
                r={5}
                fill="var(--surface)"
                stroke="var(--chart-1)"
                strokeWidth={2}
                pointerEvents="none"
              />
              {dragIdx === i && (
                <text
                  x={xAt(p.tempC)}
                  y={Math.max(yAt(p.dutyPct) - 12, PAD.top + 10)}
                  textAnchor="middle"
                  className="chart-axis-label"
                >
                  {`${p.tempC}° · ${p.dutyPct}%`}
                </text>
              )}
            </g>
          ))}
        </svg>
      )}
    </div>
  );
}
