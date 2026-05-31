/**
 * Display helpers for Forza telemetry types.
 *
 * Timestamps come from the server as int64 nanoseconds (server_recv_ns).
 * JS Date works in milliseconds — divide by 1e6.
 */

export function formatDateTime(ns: number): string {
  const ms = ns / 1_000_000;
  const d = new Date(ms);
  return d.toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function formatDate(ns: number): string {
  const ms = ns / 1_000_000;
  return new Date(ms).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "2-digit",
  });
}

export function formatTime(ns: number): string {
  const ms = ns / 1_000_000;
  return new Date(ms).toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * Duration between two NS timestamps. Returns "—" for null end (in progress)
 * or a compact "Xm Ys" / "X:YY" string.
 */
export function formatDurationNS(startNS: number, endNS: number | null): string {
  if (endNS === null) return "in progress";
  const seconds = Math.max(0, Math.round((endNS - startNS) / 1_000_000_000));
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  if (m < 60) return `${m}m ${s.toString().padStart(2, "0")}s`;
  const h = Math.floor(m / 60);
  const mm = m % 60;
  return `${h}h ${mm.toString().padStart(2, "0")}m`;
}

export function formatCount(n: number): string {
  return n.toLocaleString();
}

/**
 * Forza Data Out sends Gear as a uint8: 0 = reverse, 1..n = forward gears (it
 * does not signal neutral distinctly). Confirmed against a live reverse capture
 * — gear 0 must read "R". Single source of truth shared by the HUD + instrument.
 */
export function gearLabel(g: number): string {
  if (g === 0) return "R";
  return String(g);
}
