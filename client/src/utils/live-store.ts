import { create } from "zustand";
import type { TickFrame } from "~/types/tick.generated";

const RING_SIZE = 3600;

interface LiveState {
  connected: boolean;
  latest: TickFrame | null;
  ring: TickFrame[];
  setConnected(connected: boolean): void;
  push(tick: TickFrame): void;
  clear(): void;
}

export const useLiveStore = create<LiveState>((set, get) => ({
  connected: false,
  latest: null,
  ring: [],
  setConnected: (connected) => set({ connected }),
  push: (tick) => {
    const next = get().ring;
    next.push(tick);
    if (next.length > RING_SIZE) next.shift();
    set({ latest: tick, ring: next });
  },
  clear: () => set({ latest: null, ring: [] }),
}));
