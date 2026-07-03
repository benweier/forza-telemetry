import { create } from "zustand";
import type { TickFrame } from "~/types/tick.generated";

const RING_SIZE = 3600;

/** Calibration offset is clamped to this — the ring holds ~60 s, so 2 s of delay
 *  always has buffered ticks to read behind it. */
export const MAX_OFFSET_MS = 2000;

const LS_KEY = "forza.preview";

interface LiveState {
  connected: boolean;
  /** Wall-clock ms of the most recent push — used to detect a stale feed. */
  lastPushedAt: number | null;
  latest: TickFrame | null;
  ring: TickFrame[];
  /** Game-preview mode: hold the displayed telemetry back by `offsetMs` to match
   *  the WebRTC video's latency (see ADR 0010). Off → views render `latest`. */
  previewEnabled: boolean;
  offsetMs: number;
  /** WHEP playback URL of the MediaMTX relay. Not used by the selector — only by
   *  the preview pane — but persisted alongside the other preview config. */
  whepUrl: string;
  // Property syntax (not method syntax) — these are arrow functions with no
  // `this`, and selectors hand them out unbound (`useLiveStore(s => s.push)`).
  setConnected: (connected: boolean) => void;
  push: (tick: TickFrame) => void;
  clear: () => void;
  setPreviewEnabled: (enabled: boolean) => void;
  setOffsetMs: (ms: number) => void;
  setWhepUrl: (url: string) => void;
}

const clampOffset = (ms: number) => Math.max(0, Math.min(MAX_OFFSET_MS, Math.round(ms || 0)));

type PreviewConfig = { previewEnabled: boolean; offsetMs: number; whepUrl: string };

const DEFAULT_PREVIEW: PreviewConfig = { previewEnabled: false, offsetMs: 0, whepUrl: "" };

function loadPreview(): PreviewConfig {
  if (typeof localStorage === "undefined") return DEFAULT_PREVIEW;
  try {
    const raw = localStorage.getItem(LS_KEY);
    if (raw) {
      const p: unknown = JSON.parse(raw);
      if (typeof p === "object" && p !== null) {
        const rec: Partial<Record<keyof PreviewConfig, unknown>> = p;
        return {
          previewEnabled: !!rec.previewEnabled,
          offsetMs: clampOffset(Number(rec.offsetMs)),
          whepUrl: typeof rec.whepUrl === "string" ? rec.whepUrl : "",
        };
      }
    }
  } catch {
    // Corrupt/blocked storage — fall through to defaults.
  }
  return DEFAULT_PREVIEW;
}

function savePreview(s: PreviewConfig): void {
  if (typeof localStorage === "undefined") return;
  try {
    localStorage.setItem(
      LS_KEY,
      JSON.stringify({
        previewEnabled: s.previewEnabled,
        offsetMs: s.offsetMs,
        whepUrl: s.whepUrl,
      }),
    );
  } catch {
    // Storage full/blocked — config just won't persist; not worth surfacing.
  }
}

export const useLiveStore = create<LiveState>((set, get) => ({
  connected: false,
  lastPushedAt: null,
  latest: null,
  ring: [],
  ...loadPreview(),
  setConnected: (connected) => set({ connected }),
  push: (tick) => {
    const next = get().ring;
    next.push(tick);
    if (next.length > RING_SIZE) next.shift();
    set({ latest: tick, ring: next, lastPushedAt: Date.now() });
  },
  clear: () => set({ latest: null, ring: [], lastPushedAt: null }),
  setPreviewEnabled: (previewEnabled) => {
    set({ previewEnabled });
    savePreview(get());
  },
  setOffsetMs: (ms) => {
    set({ offsetMs: clampOffset(ms) });
    savePreview(get());
  },
  setWhepUrl: (whepUrl) => {
    set({ whepUrl });
    savePreview(get());
  },
}));

// ---------- Display selector (the one place "which tick do we show?" lives) ----------

/** Just the fields the display selector needs — keeps the pure helpers trivially
 *  testable without constructing a whole store. */
type DisplaySlice = Pick<LiveState, "ring" | "previewEnabled" | "offsetMs">;

/** Index into `ring` of the tick to display *now*. Returns the latest index when
 *  preview is off; otherwise the newest tick whose `sts` is at-or-before
 *  `(newest.sts − offset)`. Anchored to the ring's own newest timestamp, never
 *  wall-clock, so MacBook↔game-PC clock skew is irrelevant. `sts` is epoch-ns. */
export function displayIndex(s: DisplaySlice): number {
  const n = s.ring.length;
  if (n === 0) return -1;
  if (!s.previewEnabled || s.offsetMs <= 0) return n - 1;
  const target = s.ring[n - 1].sts - s.offsetMs * 1e6;
  for (let i = n - 1; i >= 0; i--) {
    if (s.ring[i].sts <= target) return i;
  }
  return 0; // offset exceeds the buffered span — show the oldest we have.
}

/** The tick to display now (delayed when preview is on), or null if the ring is empty. */
export function displayTick(s: DisplaySlice): TickFrame | null {
  const i = displayIndex(s);
  return i < 0 ? null : s.ring[i];
}

/** Reactive hook for components that subscribe to the displayed tick (re-renders
 *  per frame as the delayed pointer advances). Imperative rAF readers should call
 *  `displayTick(useLiveStore.getState())` instead. */
export function useDisplayTick(): TickFrame | null {
  return useLiveStore(displayTick);
}
