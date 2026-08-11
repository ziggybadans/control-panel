// Static system info, fetched once, plus a ticking uptime string.

import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { SystemInfo } from "../api/types";
import { fmtDuration } from "../lib/format";

let cached: SystemInfo | null = null;
const listeners = new Set<() => void>();

export function loadSystem() {
  api<{ info: SystemInfo }>("/api/system")
    .then((res) => {
      cached = res.info;
      listeners.forEach((l) => l());
    })
    .catch(() => {});
}

export function useSystem(): { info: SystemInfo | null; uptime: string } {
  const [, force] = useState(0);
  const [now, setNow] = useState(Date.now());

  useEffect(() => {
    const l = () => force((n) => n + 1);
    listeners.add(l);
    if (!cached) loadSystem();
    const t = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => {
      listeners.delete(l);
      window.clearInterval(t);
    };
  }, []);

  const uptime = cached?.bootTime ? fmtDuration(now - cached.bootTime) : "—";
  return { info: cached, uptime };
}
