// UI preferences: theme, accent, density, and dashboard layout. Persisted
// server-side (so they follow the user between machines) with localStorage as
// a fast first-paint cache.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { api } from "../api/client";

export type Theme = "dark" | "light" | "system";
export type Accent = "blue" | "teal" | "violet" | "green" | "amber" | "rose";
export type Density = "compact" | "comfortable";

export interface WidgetPref {
  id: string;
  visible: boolean;
  /** Column span: 1 = quarter, 2 = half, 4 = full row (12-col grid / 3). */
  size: 1 | 2 | 3 | 4;
}

export interface Prefs {
  version: 1;
  theme: Theme;
  accent: Accent;
  density: Density;
  widgets: WidgetPref[];
}

export const DEFAULT_WIDGETS: WidgetPref[] = [
  { id: "cpu", visible: true, size: 2 },
  { id: "memory", visible: true, size: 2 },
  { id: "network", visible: true, size: 2 },
  { id: "diskio", visible: true, size: 2 },
  { id: "system", visible: true, size: 1 },
  { id: "temps", visible: true, size: 1 },
  { id: "storage", visible: true, size: 2 },
  { id: "minecraft", visible: true, size: 2 },
  { id: "services", visible: true, size: 1 },
  { id: "plex", visible: true, size: 1 },
  { id: "apps", visible: true, size: 1 },
];

const DEFAULTS: Prefs = {
  version: 1,
  theme: "dark",
  accent: "blue",
  density: "compact",
  widgets: DEFAULT_WIDGETS,
};

function sanitize(raw: unknown): Prefs {
  const p = { ...DEFAULTS, widgets: [...DEFAULT_WIDGETS] };
  if (!raw || typeof raw !== "object") return p;
  const r = raw as Partial<Prefs>;
  if (r.theme === "light" || r.theme === "dark" || r.theme === "system") p.theme = r.theme;
  if (r.accent && ["blue", "teal", "violet", "green", "amber", "rose"].includes(r.accent))
    p.accent = r.accent;
  if (r.density === "comfortable" || r.density === "compact") p.density = r.density;
  if (Array.isArray(r.widgets)) {
    const known = new Map(DEFAULT_WIDGETS.map((w) => [w.id, w]));
    const seen: WidgetPref[] = [];
    for (const w of r.widgets) {
      if (w && typeof w.id === "string" && known.has(w.id)) {
        const size = [1, 2, 3, 4].includes(w.size as number) ? (w.size as 1 | 2 | 3 | 4) : known.get(w.id)!.size;
        seen.push({ id: w.id, visible: w.visible !== false, size });
        known.delete(w.id);
      }
    }
    // Append widgets added in newer versions of the panel.
    for (const w of known.values()) seen.push(w);
    p.widgets = seen;
  }
  return p;
}

interface PrefsContextValue {
  prefs: Prefs;
  update: (patch: Partial<Prefs>) => void;
  resetDashboard: () => void;
}

const PrefsContext = createContext<PrefsContextValue | null>(null);

function applyToDocument(p: Prefs) {
  const el = document.documentElement;
  const theme =
    p.theme === "system"
      ? window.matchMedia("(prefers-color-scheme: light)").matches
        ? "light"
        : "dark"
      : p.theme;
  el.setAttribute("data-theme", theme);
  el.setAttribute("data-accent", p.accent);
  el.setAttribute("data-density", p.density);
}

export function PrefsProvider({ children }: { children: ReactNode }) {
  const [prefs, setPrefs] = useState<Prefs>(() => {
    try {
      return sanitize(JSON.parse(localStorage.getItem("cp-prefs") ?? ""));
    } catch {
      return DEFAULTS;
    }
  });
  const saveTimer = useRef<number | undefined>(undefined);

  // Server copy wins over the local cache once loaded.
  useEffect(() => {
    api<unknown>("/api/prefs")
      .then((raw) => {
        if (raw && typeof raw === "object" && Object.keys(raw).length > 0) {
          setPrefs(sanitize(raw));
        }
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    applyToDocument(prefs);
    localStorage.setItem("cp-prefs", JSON.stringify(prefs));
    if (prefs.theme === "system") {
      const mq = window.matchMedia("(prefers-color-scheme: light)");
      const onChange = () => applyToDocument(prefs);
      mq.addEventListener("change", onChange);
      return () => mq.removeEventListener("change", onChange);
    }
  }, [prefs]);

  const persist = useCallback((next: Prefs) => {
    window.clearTimeout(saveTimer.current);
    saveTimer.current = window.setTimeout(() => {
      api("/api/prefs", { method: "PUT", body: next }).catch(() => {});
    }, 600);
  }, []);

  const update = useCallback(
    (patch: Partial<Prefs>) => {
      setPrefs((prev) => {
        const next = { ...prev, ...patch };
        persist(next);
        return next;
      });
    },
    [persist],
  );

  const resetDashboard = useCallback(() => {
    update({ widgets: DEFAULT_WIDGETS.map((w) => ({ ...w })) });
  }, [update]);

  const value = useMemo(
    () => ({ prefs, update, resetDashboard }),
    [prefs, update, resetDashboard],
  );
  return <PrefsContext.Provider value={value}>{children}</PrefsContext.Provider>;
}

export function usePrefs(): PrefsContextValue {
  const ctx = useContext(PrefsContext);
  if (!ctx) throw new Error("usePrefs outside PrefsProvider");
  return ctx;
}
