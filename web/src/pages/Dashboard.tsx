// Customizable dashboard: a 4-column grid of widgets that can be reordered
// (drag), resized, and hidden. Layout persists server-side via prefs.

import { useMemo, useState, type DragEvent } from "react";
import { usePrefs, type WidgetPref } from "../state/prefs";
import { Icon } from "../ui/Icon";
import { withViewTransition } from "../ui/motion";
import { WIDGETS } from "./widgets/registry";

export function Dashboard() {
  const { prefs, update, resetDashboard } = usePrefs();
  const [editing, setEditing] = useState(false);
  const [dragId, setDragId] = useState<string | null>(null);
  const [dropId, setDropId] = useState<string | null>(null);

  const visible = useMemo(
    () => prefs.widgets.filter((w) => w.visible && WIDGETS[w.id]),
    [prefs.widgets],
  );
  const hidden = useMemo(
    () => prefs.widgets.filter((w) => !w.visible && WIDGETS[w.id]),
    [prefs.widgets],
  );

  // Layout mutations run inside a view transition so widgets glide to
  // their new positions instead of snapping.
  function setWidgets(widgets: WidgetPref[]) {
    withViewTransition(() => update({ widgets }));
  }

  function patchWidget(id: string, patch: Partial<WidgetPref>) {
    setWidgets(prefs.widgets.map((w) => (w.id === id ? { ...w, ...patch } : w)));
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
            ? "Drag to reorder · resize with the segment buttons · hide with ×"
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

      <div className="dash-grid">
        {visible.map((w) => {
          const def = WIDGETS[w.id];
          const Comp = def.component;
          return (
            <div
              key={w.id}
              style={{ viewTransitionName: `widget-${w.id}` }}
              className={[
                `card w-${w.size}`,
                editing ? "widget-editing" : "",
                dropId === w.id && dragId !== w.id ? "widget-drop" : "",
              ]
                .filter(Boolean)
                .join(" ")}
              draggable={editing}
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
            >
              <div className="card-h">
                {editing && <Icon name="grip" size={14} className="faint" />}
                <span className="label">{def.title}</span>
                {editing ? (
                  <div className="row right">
                    <div className="choice-row" role="group" aria-label="width">
                      {([1, 2, 3, 4] as const).map((s) => (
                        <button
                          key={s}
                          className={`choice ${w.size === s ? "selected" : ""}`}
                          style={{ padding: "1px 7px", fontSize: 11 }}
                          onClick={() => patchWidget(w.id, { size: s })}
                          title={`${s}/4 width`}
                        >
                          {s}
                        </button>
                      ))}
                    </div>
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
          );
        })}
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
