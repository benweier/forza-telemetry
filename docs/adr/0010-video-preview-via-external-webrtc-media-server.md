# Video preview via external WebRTC media server, not embedded

The in-UI game-preview pane receives game video over WebRTC: OBS on the game PC pushes a WHIP stream to **MediaMTX** (a standalone WebRTC relay), and the browser plays it back via WHEP into a `<video>` element. MediaMTX runs as a separate process (a `just` recipe in dev; a `go:embed`+`exec` child supervisor at release time). The `forza-telemetry` server stays entirely out of the video path — its only video knowledge is a WHEP URL + a calibration offset, both held in browser localStorage.

## Considered Options

- **Embed pion (WebRTC-in-Go) into the server** — rejected: hundreds of lines of SDP/ICE/track-forwarding lifecycle to own and debug forever, for something a purpose-built daemon does for free. The video pipeline is orthogonal to telemetry; the telemetry server should not learn WebRTC.
- **Import MediaMTX as a library** (`core.New()`) — rejected: that surface is internal, not a stable public API; coupling to it invites churn.
- **HLS / LL-HLS instead of WebRTC** — rejected: 2–6 s latency (LL-HLS ~1–2 s, fiddly) fights both the "local Twitch" live feel and the sync calibration. WebRTC is natively a `<video>` `srcObject` with no player lib.
- **Loopback-only tricks** (OBS virtual camera + `getUserMedia`) — rejected: the viewer is not guaranteed to be on the game PC, so the stream must be LAN-reachable.

## Consequences

- Breaks the single-binary deployment story: a second process exists at runtime. Mitigated by the deferred `go:embed`+`exec` supervisor so distribution stays one file to copy.
- Adds an external, non-Go runtime dependency (MediaMTX, MIT-licensed) to an otherwise self-contained stack.
- Because the app runs on plain `http://<lan-ip>`, WHEP playback relies on `RTCPeerConnection` recvonly *not* being secure-context-gated (unlike `getUserMedia`). If that assumption fails, the LAN deployment needs HTTPS + self-signed certs. Verified by a spike before building the UI.
- Export is never the app's job: same-PC recording uses OBS compositing native game capture + a dashboard browser source (normal mode + OBS Video Delay filter); cross-machine recording screen-records the in-UI preview pane in preview+delay mode.
