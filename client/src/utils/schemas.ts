/**
 * Valibot schemas mirroring docs/api.md v1 response shapes.
 *
 * Server returns int64 nanoseconds for timestamps. JS `number` is precise to
 * 2^53, which covers nanosecond epochs through year 2255 — safe.
 *
 * Aggregate fields (stint_summary, lap_summary) are nullable because the
 * stints row is inserted on stint open with no aggregates yet; the aggregator
 * fills them at stint close.
 */
import * as v from "valibot";

// --- Primitives ---

export const NullableNumber = v.nullable(v.number());
export const NullableString = v.nullable(v.string());

// --- Sessions ---

export const SessionSchema = v.object({
  id: v.string(),
  started_at_ns: v.number(),
  ended_at_ns: NullableNumber,
  pinned: v.boolean(),
  downsampled: v.boolean(),
  stint_count: v.number(),
});

export const SessionsListResponseSchema = v.object({
  sessions: v.array(SessionSchema),
  total: v.number(),
});

// --- Stints ---

export const StintListRowSchema = v.object({
  id: v.string(),
  ordinal: v.number(),
  started_at_ns: v.number(),
  ended_at_ns: NullableNumber,
  tick_count: v.number(),
  stint_type: NullableString,
  car_ordinal: NullableNumber,
});

export const SessionDetailSchema = v.object({
  ...SessionSchema.entries,
  stints: v.array(StintListRowSchema),
});

export const CarIdentitySchema = v.object({
  ordinal: NullableNumber,
  class: NullableNumber,
  performance_index: NullableNumber,
});

export const StintSummarySchema = v.object({
  top_speed_ms: NullableNumber,
  distance_m: NullableNumber,
  avg_speed_ms: NullableNumber,
  max_rpm: NullableNumber,
  peak_lateral_g: NullableNumber,
  peak_long_g: NullableNumber,
  peak_brake_pct: NullableNumber,
  gear_shift_count: NullableNumber,
});

export const StintDetailSchema = v.object({
  id: v.string(),
  session_id: v.string(),
  ordinal: v.number(),
  started_at_ns: v.number(),
  ended_at_ns: NullableNumber,
  first_game_ts_ms: NullableNumber,
  last_game_ts_ms: NullableNumber,
  tick_count: v.number(),
  stint_type: NullableString,
  car: CarIdentitySchema,
  summary: v.nullable(StintSummarySchema),
});

// --- Sub-resources ---

export const LapSchema = v.object({
  lap_number: v.number(),
  lap_time_s: NullableNumber,
  top_speed_ms: NullableNumber,
  distance_m: NullableNumber,
  peak_lateral_g: NullableNumber,
  peak_brake_pct: NullableNumber,
});
export const LapsResponseSchema = v.object({ laps: v.array(LapSchema) });

export const HotSpotSchema = v.object({
  id: v.string(),
  type: v.string(),
  started_at_ns: v.number(),
  ended_at_ns: v.number(),
  peak_tick_ns: v.number(),
  peak_value: v.number(),
  label: v.string(),
  // Per ADR 0008, each hot-spot belongs to exactly one Turn or one Straight
  // (XOR enforced server-side). Both null on legacy rows, never both non-null.
  turn_id: NullableString,
  straight_id: NullableString,
});
export const HotSpotsResponseSchema = v.object({ hot_spots: v.array(HotSpotSchema) });

export const TurnSchema = v.object({
  id: v.string(),
  turn_index: v.number(),
  started_at_ns: v.number(),
  apex_tick_ns: v.number(),
  ended_at_ns: v.number(),
  peak_curvature: v.number(),
  peak_delta_theta: v.number(),
  direction: v.string(),
  shape: NullableString,
});
export const TurnsResponseSchema = v.object({ turns: v.array(TurnSchema) });

export const StraightSchema = v.object({
  id: v.string(),
  straight_index: v.number(),
  started_at_ns: v.number(),
  ended_at_ns: v.number(),
  distance_m: v.number(),
  peak_speed_ms: NullableNumber,
});
export const StraightsResponseSchema = v.object({ straights: v.array(StraightSchema) });

export const PreviewSampleSchema = v.object({
  second_index: v.number(),
  tick_ns: v.number(),
  speed_ms: NullableNumber,
  lateral_g: NullableNumber,
  longitudinal_g: NullableNumber,
  throttle_pct: NullableNumber,
  brake_pct: NullableNumber,
  rpm: NullableNumber,
  pos_x: NullableNumber,
  pos_y: NullableNumber,
  pos_z: NullableNumber,
  lap_number: NullableNumber,
});
export const PreviewResponseSchema = v.object({ samples: v.array(PreviewSampleSchema) });

// /stints/{id}/path — column-oriented downsampled XYZ + speed for 3D rendering.
// Columns are fixed server-side: [server_recv_ns, pos_x, pos_y, pos_z, speed_ms].
// `step` is the parquet row stride; `sample_hz` is 60 / step.
export const PathResponseSchema = v.object({
  columns: v.array(v.string()),
  rows: v.array(v.array(v.nullable(v.number()))),
  step: v.number(),
  sample_hz: v.number(),
});

// --- Inferred types ---

export type Session = v.InferOutput<typeof SessionSchema>;
export type SessionsListResponse = v.InferOutput<typeof SessionsListResponseSchema>;
export type StintListRow = v.InferOutput<typeof StintListRowSchema>;
export type SessionDetail = v.InferOutput<typeof SessionDetailSchema>;
export type StintDetail = v.InferOutput<typeof StintDetailSchema>;
export type StintSummary = v.InferOutput<typeof StintSummarySchema>;
export type CarIdentity = v.InferOutput<typeof CarIdentitySchema>;
export type Lap = v.InferOutput<typeof LapSchema>;
export type HotSpot = v.InferOutput<typeof HotSpotSchema>;
export type Turn = v.InferOutput<typeof TurnSchema>;
export type Straight = v.InferOutput<typeof StraightSchema>;
export type PreviewSample = v.InferOutput<typeof PreviewSampleSchema>;
export type PathResponse = v.InferOutput<typeof PathResponseSchema>;
