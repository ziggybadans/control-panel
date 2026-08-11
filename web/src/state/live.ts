// Live data layer: one EventSource multiplexes all server push streams into
// small external stores consumed via useSyncExternalStore. Chart-heavy
// components subscribe to exactly the slice they render, so a 1 Hz metrics
// tick re-renders only the widgets that display it.

import { useSyncExternalStore } from "react";
import type { Job, MCServer, Service, Snapshot } from "../api/types";
import { api } from "../api/client";

const HISTORY_MAX = 600;

class Store<T> {
  private value: T;
  private listeners = new Set<() => void>();
  constructor(initial: T) {
    this.value = initial;
  }
  get = (): T => this.value;
  set(v: T) {
    this.value = v;
    this.listeners.forEach((l) => l());
  }
  subscribe = (l: () => void): (() => void) => {
    this.listeners.add(l);
    return () => this.listeners.delete(l);
  };
}

export type ConnState = "connecting" | "live" | "reconnecting";

// History is kept in a mutable ring; the store value is a monotonically
// increasing revision so subscribers re-read the ring on change.
const history: Snapshot[] = [];
const historyRev = new Store(0);
const latest = new Store<Snapshot | null>(null);
const services = new Store<Service[]>([]);
const mcServers = new Store<MCServer[]>([]);
const jobs = new Store<Job[]>([]);
const conn = new Store<ConnState>("connecting");

function pushSnapshot(s: Snapshot) {
  history.push(s);
  if (history.length > HISTORY_MAX) history.splice(0, history.length - HISTORY_MAX);
  latest.set(s);
  historyRev.set(historyRev.get() + 1);
}

let source: EventSource | null = null;
let started = false;

export function startLive() {
  if (started) return;
  started = true;

  // Fill charts immediately instead of waiting for ticks to accumulate.
  api<Snapshot[]>("/api/metrics/history")
    .then((h) => {
      if (h && h.length) {
        history.splice(0, history.length, ...h.slice(-HISTORY_MAX));
        latest.set(history[history.length - 1] ?? null);
        historyRev.set(historyRev.get() + 1);
      }
    })
    .catch(() => {});

  connect();
}

export function stopLive() {
  started = false;
  source?.close();
  source = null;
}

function connect() {
  source = new EventSource("/api/events");
  source.onopen = () => conn.set("live");
  source.onerror = () => {
    // EventSource reconnects on its own; just reflect the state.
    conn.set("reconnecting");
  };
  source.addEventListener("metrics", (e) => {
    pushSnapshot(JSON.parse((e as MessageEvent).data));
  });
  source.addEventListener("services", (e) => {
    services.set(JSON.parse((e as MessageEvent).data) ?? []);
  });
  source.addEventListener("mc", (e) => {
    mcServers.set(JSON.parse((e as MessageEvent).data) ?? []);
  });
  source.addEventListener("jobs", (e) => {
    jobs.set(JSON.parse((e as MessageEvent).data) ?? []);
  });
}

// --- hooks ------------------------------------------------------------------

export function useLatestMetrics(): Snapshot | null {
  return useSyncExternalStore(latest.subscribe, latest.get);
}

/** Returns the shared history ring; identity changes are signaled via rev. */
export function useMetricsHistory(): { history: Snapshot[]; rev: number } {
  const rev = useSyncExternalStore(historyRev.subscribe, historyRev.get);
  return { history, rev };
}

export function useServices(): Service[] {
  return useSyncExternalStore(services.subscribe, services.get);
}

export function useMCServers(): MCServer[] {
  return useSyncExternalStore(mcServers.subscribe, mcServers.get);
}

export function useMCServer(id: string): MCServer | undefined {
  const all = useMCServers();
  return all.find((s) => s.id === id);
}

export function useJobs(): Job[] {
  return useSyncExternalStore(jobs.subscribe, jobs.get);
}

export function useConnState(): ConnState {
  return useSyncExternalStore(conn.subscribe, conn.get);
}
