# HTTP API (v1)

All endpoints under `/api/v1/`. JSON only. Timestamps are signed int64
nanoseconds since Unix epoch (`server_recv_ns`). Snake_case throughout.

## Versioning

The `v1` prefix is the API contract. Breaking changes — renamed fields,
removed endpoints, changed status codes for known cases — go behind a `v2`
prefix; `v1` keeps responding the same way until removed. Additive changes
(new fields, new endpoints) ship under `v1` without notice.

The Tick wire schema (`tick.Tick` superset, ADR 0003) evolves additively
forever. The REST schema below evolves additively under `v1`.

## Sessions

### `GET /api/v1/sessions`
List all sessions, newest first.

```json
{
  "sessions": [
    {
      "id": "20260524T170000Z",
      "started_at_ns": 1779609381000000000,
      "ended_at_ns": 1779609395000000000,   // null while in progress
      "pinned": false,
      "downsampled": false,
      "stint_count": 3
    }
  ],
  "total": 1
}
```

### `GET /api/v1/sessions/{id}`
Session detail with stint list. `404` if no such session.

```json
{
  "id": "20260524T170000Z",
  "started_at_ns": 1779609381000000000,
  "ended_at_ns": 1779609395000000000,
  "pinned": false,
  "downsampled": false,
  "stint_count": 3,
  "stints": [
    {
      "id": "20260524T170000Z_0001",
      "ordinal": 1,
      "started_at_ns": 1779609381100000000,
      "ended_at_ns": 1779609390000000000,
      "tick_count": 480,
      "stint_type": "freeroam",   // null until close-time resolution
      "car_ordinal": 3773
    }
  ]
}
```

### `PATCH /api/v1/sessions/{id}`
Toggle the `pinned` flag (only mutable field). Body:

```json
{ "pinned": true }
```

Returns the updated session detail (same shape as `GET`). `400` for missing
or unknown fields, `404` if session not found.

### `POST /api/v1/sessions/{id}/downsample`
**Currently returns `501 Not Implemented`** — endpoint shape pinned so the UI
can wire its affordance. Real Parquet rewrite lands in a later pass.

```json
{
  "error": "downsample action not yet implemented",
  "note":  "the endpoint shape is stable; the Parquet rewrite job is not built yet"
}
```

### `DELETE /api/v1/sessions/{id}`
Delete a session and everything beneath it (stints, child rows, Parquet
files). `404` if no such session; `409` if the session is still recording:

```json
{ "error": "cannot delete a session that is still recording" }
```

Success returns `200`:

```json
{ "deleted": "20260524T170000Z" }
```

## Stints

### `GET /api/v1/stints/{id}`
Full stint detail with embedded aggregate summary.

```json
{
  "id": "20260524T170000Z_0001",
  "session_id": "20260524T170000Z",
  "ordinal": 1,
  "started_at_ns": 1779609381100000000,
  "ended_at_ns": 1779609390000000000,
  "first_game_ts_ms": 0,
  "last_game_ts_ms": 8900,
  "tick_count": 480,
  "stint_type": "circuit",
  "car": {
    "ordinal": 3773,
    "class": 3,
    "performance_index": 700
  },
  "summary": {
    "top_speed_ms": 50.0,
    "distance_m": 1500.0,
    "avg_speed_ms": 25.0,
    "max_rpm": 6500.0,
    "peak_lateral_g": 1.2,
    "peak_long_g": 0.6,
    "peak_brake_pct": 0.85,
    "gear_shift_count": 3
  }
}
```

`summary` is `null` until aggregation runs (it runs immediately at stint
close, so live-in-progress stints will have `null` summary).

Note: the Parquet file path is **never** exposed — it is a server filesystem
detail. Tick data is fetched through the tick-series endpoint below.

### `GET /api/v1/stints/{id}/laps`
Per-lap summaries.

```json
{
  "laps": [
    {
      "lap_number": 0,
      "lap_time_s": 78.5,
      "top_speed_ms": 50.0,
      "distance_m": 1500.0,
      "peak_lateral_g": 1.2,
      "peak_brake_pct": 0.85
    }
  ]
}
```

### `DELETE /api/v1/stints/{id}`
Delete a single stint (child rows + Parquet file). `404` if no such stint;
`409` (`{"error": "cannot delete a stint that is still recording"}`) if the
stint is still recording. Success returns `200 {"deleted": "<id>"}`.

### `GET /api/v1/stints/{id}/preview`
1Hz preview series for the scrub bar. One row per second of stint wall time.

```json
{
  "samples": [
    {
      "second_index": 0,
      "tick_ns": 1779609381100000000,
      "speed_ms": 25.0,
      "lateral_g": 0.5,
      "longitudinal_g": 0.2,
      "throttle_pct": 0.6,
      "brake_pct": 0.0,
      "rpm": 5500.0,
      "pos_x": 100.0,
      "pos_y": 50.0,
      "pos_z": 200.0,
      "lap_number": 0
    }
  ]
}
```

### `GET /api/v1/stints/{id}/ticks?from&to&channels`
Full-resolution tick series, column-oriented. Read directly from Parquet.

**Query params:**
- `from` (ns, optional) — default: stint start.
- `to` (ns, optional) — default: `from + 60s`.
- `channels` (csv, optional) — default: `speed_ms,engine_rpm,throttle_pct,brake_pct,lateral_g,longitudinal_g,gear,lap_number`.

**Limits:**
- `to - from` must be ≤ **60 seconds**. Larger windows return `400`. Page by
  issuing multiple requests with disjoint `from/to`.
- `channels` must be a subset of the whitelist (`ticks.go tickChannels`).
  Unknown names return `400`.
- The actively-recording stint returns `409` (`{"error": "stint is still
  recording"}`) — its Parquet file has no footer until the stint closes.

**Response:** column-oriented, optimal for chart libs that take parallel arrays.

```json
{
  "from_ns": 1779609381100000000,
  "to_ns":   1779609386100000000,
  "columns": ["server_recv_ns", "speed_ms", "engine_rpm"],
  "rows": [
    [1779609381100000000, 10.0, 5000.0],
    [1779609381116666666, 10.5, 5020.0]
  ]
}
```

`server_recv_ns` is always the first column.

### `GET /api/v1/stints/{id}/path?step`
Downsampled 3D track path for map rendering, column-oriented like `/ticks`
with a fixed column set: `server_recv_ns, pos_x, pos_y, pos_z, speed_ms,
lap_number, brake_pct, lateral_g`.

**Query params:**
- `step` (int, optional, default `6`, max `60`) — keep every Nth tick
  (`step=6` ≈ 10 Hz at Forza's 60 Hz output).

Returns `404` for an unknown stint, `409` while the stint is still recording.

```json
{
  "columns": ["server_recv_ns", "pos_x", "pos_y", "pos_z", "speed_ms", "lap_number", "brake_pct", "lateral_g"],
  "rows": [[1779609381100000000, 100.0, 50.0, 200.0, 25.0, 0, 0.0, 0.5]],
  "step": 6,
  "sample_hz": 10.0
}
```

## Live channel

### `GET /api/v1/live` (WebSocket)

Server → client only today. Every frame — HELLO included — is a
MessagePack-encoded envelope:

- `k=1` (HELLO) — sent once on connect (protocol version).
- `k=2` (TICK) — payload under `t` is one Tick. Field names are short tags
  (`gv`, `pv`, `gts`, `sp`, …) per `client/src/types/tick.generated.ts`.

See `types/tick.generated.ts` for the full Tick field set + enum values.

## Health

### `GET /healthz`
Returns `ok` (text/plain).

## Errors

Non-2xx responses are JSON `{"error": "..."}` with the relevant status code.
Server-side failures surface as `500 internal error` and detail is logged
rather than returned.
