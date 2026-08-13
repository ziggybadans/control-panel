// qBittorrent display helpers, shared by the qBittorrent page and the Plex
// dashboard widget.

import type { QbitTorrent, QbitWatch } from "../api/types";
import { fmtDuration } from "./format";

/** States where bytes are still expected to arrive. */
export function isDownloading(state: string): boolean {
  switch (state) {
    case "downloading":
    case "forcedDL":
    case "metaDL":
    case "stalledDL":
    case "queuedDL":
    case "checkingDL":
    case "allocating":
      return true;
  }
  return false;
}

export type Tone = "ok" | "warn" | "crit" | "neutral";

/** Short label and severity for a qBittorrent state string. */
export function stateBadge(state: string): { label: string; tone: Tone } {
  switch (state) {
    case "downloading":
    case "forcedDL":
      return { label: "downloading", tone: "ok" };
    case "uploading":
    case "forcedUP":
      return { label: "seeding", tone: "ok" };
    case "stalledDL":
      return { label: "stalled", tone: "warn" };
    case "stalledUP":
      return { label: "seeding idle", tone: "neutral" };
    case "metaDL":
      return { label: "metadata", tone: "warn" };
    case "allocating":
      return { label: "allocating", tone: "neutral" };
    case "checkingDL":
    case "checkingUP":
    case "checkingResumeData":
      return { label: "checking", tone: "warn" };
    case "moving":
      return { label: "moving", tone: "warn" };
    case "queuedDL":
    case "queuedUP":
      return { label: "queued", tone: "neutral" };
    case "pausedDL":
    case "stoppedDL":
      return { label: "paused", tone: "neutral" };
    case "pausedUP":
    case "stoppedUP":
      return { label: "done", tone: "neutral" };
    case "error":
    case "missingFiles":
      return { label: state === "error" ? "error" : "files missing", tone: "crit" };
    default:
      return { label: state, tone: "neutral" };
  }
}

/**
 * The "can I watch it yet?" badge. The verdict compares how long the rest
 * of the download takes against how long the media plays for — see the Go
 * side (internal/qbit) for the arithmetic.
 */
export function watchBadge(w: QbitWatch): {
  label: string;
  tone: Tone | "muted";
  title: string;
} {
  const eta = w.etaSec ? fmtDuration(w.etaSec * 1000) : "—";
  const runtime = w.runtimeSec ? fmtDuration(w.runtimeSec * 1000) : "—";
  const caveat = w.sequential
    ? ""
    : " Sequential download is off, so pieces arrive out of order and a partial file cannot be played — turn it on for this torrent first.";
  switch (w.verdict) {
    case "ready":
      return {
        label: "ready",
        tone: "ok",
        title: "Download complete — it plays once the *arr import lands in Plex.",
      };
    case "now":
      return {
        label: "stream now",
        tone: "ok",
        title:
          `${eta} left at the current rate against ${runtime} of runtime: ` +
          `the download stays ahead of playback (10% cushion).${caveat}`,
      };
    case "wait":
      return {
        label: `wait ${fmtDuration((w.waitSec ?? 0) * 1000)}`,
        tone: "warn",
        title:
          `${eta} left at the current rate but only ${runtime} of runtime: ` +
          `starting now, playback would catch up. Wait ${fmtDuration((w.waitSec ?? 0) * 1000)}.${caveat}`,
      };
    case "stalled":
      return {
        label: "stalled",
        tone: "warn",
        title: "No download rate, so there is nothing to estimate from.",
      };
    case "paused":
      return { label: "paused", tone: "neutral", title: "Torrent is paused." };
    case "queued":
      return {
        label: "queued",
        tone: "neutral",
        title: "Waiting behind other torrents in the qBittorrent queue.",
      };
    default:
      return {
        label: "—",
        tone: "muted",
        title:
          "No Radarr/Sonarr queue item matches this torrent, so its runtime " +
          "is unknown and playback can't be compared against the download.",
      };
  }
}

/** Title to show for a torrent: the matched media, else the release name. */
export function torrentTitle(t: QbitTorrent): string {
  return t.media || t.name;
}
