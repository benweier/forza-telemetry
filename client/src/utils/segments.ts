/**
 * Hot-spot ↔ Segment grouping per ADR 0008.
 *
 * The server attributes each hot-spot to exactly one Turn or one Straight
 * (XOR-enforced). For UI rendering we collapse to one event per
 * (segment, type), keeping the peak with the highest peak_value. This
 * preserves per-place visibility (every detected segment surfaces what
 * happened there) while suppressing the noise of multiple peaks of the
 * same kind clustered around one corner.
 */
import type { HotSpot, Straight, Turn } from "~/utils/schemas";

export interface GroupedEvent {
  hotSpot: HotSpot;
  segmentLabel: string; // "Turn 3" / "Straight 5"
  segmentStartNS: number; // for chronological sorting
  isTurn: boolean;
  segmentIndex: number;
}

/**
 * Group hot-spots into one event per (segment, type), winner-by-peak-value.
 * Output is sorted chronologically by segment start, then by type so events
 * within the same segment read in a stable order.
 */
export function groupHotSpotsBySegment(
  hotSpots: HotSpot[],
  turns: Turn[],
  straights: Straight[],
): GroupedEvent[] {
  const turnByID = new Map(turns.map((t) => [t.id, t]));
  const straightByID = new Map(straights.map((s) => [s.id, s]));

  const best = new Map<string, HotSpot>();
  for (const h of hotSpots) {
    const segmentID = h.turn_id ?? h.straight_id;
    if (!segmentID) continue;
    const key = `${segmentID}::${h.type}`;
    const prev = best.get(key);
    if (!prev || h.peak_value > prev.peak_value) {
      best.set(key, h);
    }
  }

  const out: GroupedEvent[] = [];
  for (const h of best.values()) {
    if (h.turn_id) {
      const t = turnByID.get(h.turn_id);
      if (!t) continue;
      out.push({
        hotSpot: h,
        segmentLabel: `Turn ${t.turn_index}`,
        segmentStartNS: t.started_at_ns,
        isTurn: true,
        segmentIndex: t.turn_index,
      });
    } else if (h.straight_id) {
      const s = straightByID.get(h.straight_id);
      if (!s) continue;
      out.push({
        hotSpot: h,
        segmentLabel: `Straight ${s.straight_index}`,
        segmentStartNS: s.started_at_ns,
        isTurn: false,
        segmentIndex: s.straight_index,
      });
    }
  }

  out.sort((a, b) => {
    if (a.segmentStartNS !== b.segmentStartNS) {
      return a.segmentStartNS - b.segmentStartNS;
    }
    return a.hotSpot.type.localeCompare(b.hotSpot.type);
  });
  return out;
}

/**
 * Type label + colour used by both the marker layer and the events panel.
 * Keeping them in one place ensures the legend matches the map.
 */
export function hotSpotTypeLabel(t: string): string {
  switch (t) {
    case "peak_lateral_g": {
      return "Lateral G peak";
    }
    case "peak_brake": {
      return "Brake peak";
    }
    case "top_speed": {
      return "Top speed";
    }
    default: {
      return t;
    }
  }
}

/**
 * RGB colour for a hot-spot type. Returned as a CSS rgb() string so consumers
 * can drop it into inline styles. The deck.gl marker layer reads the same
 * mapping in its own [r,g,b,a] form (kept synchronised by convention).
 */
export function hotSpotTypeColor(t: string): string {
  switch (t) {
    case "peak_lateral_g": {
      return "rgb(255, 200, 60)"; // amber — cornering
    }
    case "peak_brake": {
      return "rgb(240, 80, 80)"; // red — braking
    }
    case "top_speed": {
      return "rgb(80, 180, 255)"; // blue — speed
    }
    default: {
      return "rgb(200, 200, 200)";
    }
  }
}
