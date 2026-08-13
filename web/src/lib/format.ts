// Formatting helpers. All sizes are IEC (KiB/MiB/GiB/TiB) — this is a NAS
// panel, drives lie enough already.

const IEC = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];

export function fmtBytes(n: number | undefined, digits = 1): string {
  if (n === undefined || !isFinite(n) || n < 0) return "—";
  if (n === 0) return "0 B";
  const i = Math.min(Math.floor(Math.log2(n) / 10), IEC.length - 1);
  const v = n / 2 ** (10 * i);
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : digits)} ${IEC[i]}`;
}

/** Network/disk rates in decimal bits or bytes per second. */
export function fmtRate(bytesPerSec: number | undefined): string {
  if (bytesPerSec === undefined || !isFinite(bytesPerSec)) return "—";
  if (bytesPerSec < 1) return "0 B/s";
  const units = ["B/s", "KB/s", "MB/s", "GB/s"];
  const i = Math.min(Math.floor(Math.log10(bytesPerSec) / 3), units.length - 1);
  const v = bytesPerSec / 10 ** (3 * i);
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
}

export function fmtPct(v: number | undefined, digits = 0): string {
  if (v === undefined || !isFinite(v)) return "—";
  return `${v.toFixed(digits)}%`;
}

export function fmtDuration(ms: number): string {
  if (!isFinite(ms) || ms < 0) return "—";
  const s = Math.floor(ms / 1000);
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s % 60}s`;
  return `${s}s`;
}

export function fmtUptimeSince(startMs: number | undefined): string {
  if (!startMs) return "—";
  return fmtDuration(Date.now() - startMs);
}

export function fmtTime(ms: number): string {
  return new Date(ms).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export function fmtDateTime(ms: number): string {
  const d = new Date(ms);
  return `${d.toLocaleDateString([], {
    year: "numeric",
    month: "short",
    day: "numeric",
  })} ${d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`;
}

export function fmtRelative(ms: number): string {
  const delta = Date.now() - ms;
  if (delta < -60_000) return "in " + fmtSpan(-delta);
  if (delta < 60_000) return "just now";
  return fmtSpan(delta) + " ago";
}

function fmtSpan(delta: number): string {
  if (delta < 3_600_000) return `${Math.floor(delta / 60_000)}m`;
  if (delta < 86_400_000) return `${Math.floor(delta / 3_600_000)}h`;
  return `${Math.floor(delta / 86_400_000)}d`;
}

export function fmtTemp(c: number | undefined): string {
  if (c === undefined || !isFinite(c) || c === 0) return "—";
  return `${Math.round(c)}°C`;
}

/** Capacity severity for bars: ok < 85% <= warn < 95% <= crit. */
export function capacityLevel(used: number, total: number): "" | "warn" | "crit" {
  if (!total) return "";
  const pct = (used / total) * 100;
  if (pct >= 95) return "crit";
  if (pct >= 85) return "warn";
  return "";
}
