// Customizable dashboard: a 4-column masonry grid of widgets. Cards size to
// their content by default; the corner grabber drag-resizes width (snapping
// to columns) and height (free px, per-widget minimum — double-click for
// auto). Reorder (drag) and hide live in edit mode. Layout persists
// server-side via prefs.

import {
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type DragEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import { usePrefs, type WidgetPref } from "../state/prefs";
import { Icon } from "../ui/Icon";
import { withViewTransition } from "../ui/motion";
import { WIDGETS, type WidgetDef } from "./widgets/registry";

/** Height of one masonry grid row in px (grid-auto-rows in CSS). */
const ROW = 8;
const MAX_H = 1400;

export function Dashboard() {
  const { prefs, update, resetDashboard } = usePrefs();
  const [editing, setEditing] = useState(false);
  const [dragId, setDragId] = useState<string | null>(null);
  const [dropId, setDropId] = useState<string | null>(null);
  const gridRef = useRef<HTMLDivElement>(null);

  const visible = useMemo(
    () => prefs.widgets.filter((w) => w.visible && WIDGETS[w.id]),
    [prefs.widgets],
  );
  const hidden = useMemo(
    () => prefs.widgets.filter((w) => !w.visible && WIDGETS[w.id]),
    [prefs.widgets],
  );

  // Reorder/hide/reset animate with a view transition; resize must not
  // (it fires every pointer move).
  function setWidgets(widgets: WidgetPref[], animate = true) {
    if (animate) {
      withViewTransition(() => update({ widgets }));
    } else {
      update({ widgets });
    }
  }

  function patchWidget(id: string, patch: Partial<WidgetPref>, animate = true) {
    setWidgets(
      prefs.widgets.map((w) => (w.id === id ? { ...w, ...patch } : w)),
      animate,
    );
  }

  function moveWidget(from: string, to: string) {
    if (from === to) return;
    const list = [...prefs.widgets];
    const fi = list.findIndex((w) => w.id === from);
    const ti = list.findIndex((w) => w.id === to);
    if (fi < 0 || ti < 0) return;
    const [item] = list.splice(fi, 1);
    list.splice(ti, 0, item);
    setWidgets(list);
  }

  function onDrop(e: DragEvent, targetId: string) {
    e.preventDefault();
    if (dragId) moveWidget(dragId, targetId);
    setDragId(null);
    setDropId(null);
  }

  return (
    <div className="page">
      <div className="row">
        <span className="small muted">
          {editing
            ? "Drag cards to reorder · hide with × · resize anytime with the corner grabber"
            : ""}
        </span>
        <div className="row right">
          {editing && (
            <button className="btn btn-sm" onClick={resetDashboard}>
              Reset layout
            </button>
          )}
          <button
            className={editing ? "btn btn-sm btn-primary" : "btn btn-sm"}
            onClick={() => setEditing((v) => !v)}
          >
            <Icon name="edit" size={13} />
            {editing ? "Done" : "Customise"}
          </button>
        </div>
      </div>

      <div className="dash-grid" ref={gridRef}>
        {visible.map((w) => (
          <WidgetCard
            key={w.id}
            w={w}
            def={WIDGETS[w.id]}
            editing={editing}
            dropTarget={dropId === w.id && dragId !== w.id}
            gridRef={gridRef}
            patchWidget={patchWidget}
            onDragStart={() => setDragId(w.id)}
            onDragEnd={() => {
              setDragId(null);
              setDropId(null);
            }}
            onDragOver={(e) => {
              if (editing && dragId) {
                e.preventDefault();
                setDropId(w.id);
              }
            }}
            onDrop={(e) => onDrop(e, w.id)}
          />
        ))}
      </div>

      {editing && hidden.length > 0 && (
        <div className="card fade-in-up">
          <div className="card-h">
            <span className="label">Hidden widgets</span>
          </div>
          <div className="card-b row wrap">
            {hidden.map((w) => (
              <button
                key={w.id}
                className="btn btn-sm"
                onClick={() => patchWidget(w.id, { visible: true })}
              >
                <Icon name="plus" size={12} />
                {WIDGETS[w.id].title}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function WidgetCard({
  w,
  def,
  editing,
  dropTarget,
  gridRef,
  patchWidget,
  onDragStart,
  onDragEnd,
  onDragOver,
  onDrop,
}: {
  w: WidgetPref;
  def: WidgetDef;
  editing: boolean;
  dropTarget: boolean;
  gridRef: React.RefObject<HTMLDivElement>;
  patchWidget: (id: string, patch: Partial<WidgetPref>, animate?: boolean) => void;
  onDragStart: () => void;
  onDragEnd: () => void;
  onDragOver: (e: DragEvent) => void;
  onDrop: (e: DragEvent) => void;
}) {
  const innerRef = useRef<HTMLDivElement>(null);
  const [contentH, setContentH] = useState<number | null>(null);
  const [liveH, setLiveH] = useState<number | null>(null); // during a resize drag
  const [rowGap, setRowGap] = useState(14);
  const resize = useRef<{
    startX: number;
    startY: number;
    startH: number;
    startSize: number;
    colW: number;
    gap: number;
    cols: number;
    h: number;
  } | null>(null);

  const minH = def.minH ?? 96;
  const fixedH = liveH ?? (w.h && w.h > 0 ? w.h : undefined);

  // Auto mode: the masonry span follows the content's natural height.
  useLayoutEffect(() => {
    const el = innerRef.current;
    if (!el) return;
    const parent = el.parentElement?.parentElement; // the grid
    if (parent) {
      setRowGap(parseFloat(getComputedStyle(parent).rowGap) || 14);
    }
    const ro = new ResizeObserver((entries) => {
      setContentH(entries[0].contentRect.height);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // +2 covers the card border (heights are border-box).
  const effH = fixedH ?? (contentH !== null ? contentH + 2 : 120);
  const span = Math.max(1, Math.ceil((effH + rowGap) / (ROW + rowGap)));
  const Comp = def.component;

  function onGrabberDown(e: ReactPointerEvent) {
    e.preventDefault();
    e.stopPropagation();
    const grid = gridRef.current;
    const card = innerRef.current?.parentElement;
    if (!grid || !card) return;
    const cs = getComputedStyle(grid);
    const cols = cs.gridTemplateColumns.split(" ").length;
    const gap = parseFloat(cs.columnGap) || 14;
    const colW = (grid.getBoundingClientRect().width - (cols - 1) * gap) / cols;
    const startH = card.getBoundingClientRect().height;
    resize.current = {
      startX: e.clientX,
      startY: e.clientY,
      startH,
      startSize: w.size,
      colW,
      gap,
      cols,
      h: startH,
    };
    (e.target as Element).setPointerCapture(e.pointerId);
  }

  function onGrabberMove(e: ReactPointerEvent) {
    const r = resize.current;
    if (!r) return;
    r.h = Math.min(MAX_H, Math.max(minH, Math.round(r.startH + e.clientY - r.startY)));
    setLiveH(r.h);
    const wantPx = r.startSize * r.colW + (r.startSize - 1) * r.gap + (e.clientX - r.startX);
    const size = Math.min(r.cols, Math.max(1, Math.round((wantPx + r.gap) / (r.colW + r.gap))));
    if (size !== w.size) {
      patchWidget(w.id, { size: size as WidgetPref["size"] }, false);
    }
  }

  function onGrabberUp() {
    const r = resize.current;
    if (!r) return;
    resize.current = null;
    setLiveH(null);
    patchWidget(w.id, { h: r.h }, false);
  }

  return (
    <div
      style={{
        viewTransitionName: `widget-${w.id}`,
        gridRowEnd: `span ${span}`,
        height: fixedH,
      }}
      className={[
        `card w-${w.size}`,
        fixedH !== undefined ? "h-fixed" : "",
        editing ? "widget-editing" : "",
        dropTarget ? "widget-drop" : "",
      ]
        .filter(Boolean)
        .join(" ")}
      draggable={editing}
      onDragStart={(e) => {
        if ((e.target as HTMLElement).closest?.(".widget-grabber")) {
          e.preventDefault();
          return;
        }
        onDragStart();
      }}
      onDragEnd={onDragEnd}
      onDragOver={onDragOver}
      onDrop={onDrop}
    >
      <div className="widget-inner" ref={innerRef}>
        <div className="card-h">
          {editing && <Icon name="grip" size={14} className="faint" />}
          <span className="label">{def.title}</span>
          {editing ? (
            <div className="row right">
              <button
                className="btn btn-ghost btn-sm btn-icon"
                onClick={() => patchWidget(w.id, { visible: false })}
                title="Hide widget"
              >
                <Icon name="x" size={13} />
              </button>
            </div>
          ) : (
            def.headerExtra && <def.headerExtra />
          )}
        </div>
        <div className="card-b grow">
          <Comp />
        </div>
      </div>
      <div
        className="widget-grabber"
        title="Drag to resize · double-click for automatic height"
        onPointerDown={onGrabberDown}
        onPointerMove={onGrabberMove}
        onPointerUp={onGrabberUp}
        onDoubleClick={() => patchWidget(w.id, { h: 0 }, false)}
      />
    </div>
  );
}
