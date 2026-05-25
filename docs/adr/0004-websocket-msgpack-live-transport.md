# WebSocket + MessagePack for live transport

The browser ↔ Go server live channel is a single WebSocket connection carrying a structured envelope: JSON for control messages (subscribe, scrub, snapshot), MessagePack for **Tick** frames. One transport, one connection per tab, bidirectional from day one.

## Considered Options

- SSE for telemetry + REST for control — rejected: two transports; control messages would incur HTTP round-trip latency for scrub/pause actions.
- gRPC-web streaming — rejected: heavy toolchain for a single-user LAN app.
- WebTransport — rejected: unnecessary for LAN bandwidth; Go server libs and browser support still maturing.

## Consequences

- Frontend implements ~30-line reconnect helper (WebSocket lacks SSE's built-in auto-reconnect).
- MessagePack-encoded Ticks compress ~3× vs JSON and decode noticeably faster in JS — material at 60 Hz live render.
- Same envelope format serves live streaming and historical playback control.
