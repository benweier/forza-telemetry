# Plan — In-UI game preview (live, WebRTC)

Watch the game video side-by-side with the live dashboard in the browser, synced via a
manual offset. Live-only. The app never owns video export (see ADR 0010).

## Step 0 — Spike: WHEP over plain http on the LAN (BLOCKING) — ✅ PASSED

WHEP playback confirmed in a phone browser over plain `http://<lan-ip>` (OBS WHIP →
MediaMTX → browser). `secureContext=false` was a non-issue; `RTCPeerConnection` recvonly
is not secure-context-gated. No HTTPS detour needed. Spike kept at `spike/whep.html`.

<details><summary>Original spike instructions</summary>

Before any React work, confirm the transport assumption.

- Run MediaMTX locally; push a test WHIP stream from OBS (WHIP output, `http://<host>:8889/whip/test`).
- From a **phone/tablet on the LAN** (not localhost), load a ~20-line static HTML page served
  over plain `http://<lan-ip>` that opens an `RTCPeerConnection` (recvonly), POSTs the SDP offer
  to MediaMTX's WHEP endpoint, sets the answer, and attaches the track to a `<video>`.
- **Pass** = video plays. **Fail** (secure-context error) = stop and decide on HTTPS + self-signed
  certs for the LAN before continuing — that changes the deployment story.

Throwaway code. Its only output is a yes/no.
</details>

## Step 1 — `just` recipe for MediaMTX — ✅ DONE

- Added a `media` recipe (binary overridable via `MEDIAMTX=…`) and a `dev-local` recipe
  (server + client + relay, for the all-on-one-box topology C).
- **Config:** ships a 2-line `mediamtx.yml` (`paths: all_others:`) passed via
  `{{justfile_directory()}}/mediamtx.yml`. MediaMTX's *empty* config has no catch-all path and
  rejects publishing ("path 'mystream' is not configured" → OBS shows a stream-key error). All
  other settings (WebRTC :8889, no auth) stay default. (Initially shipped no config — wrong; the
  empty default doesn't permit arbitrary paths in v1.19.)
- **Deviation 2:** NOT folded into `just dev`. In the common split topology the relay lives on
  the game PC, not the dev box, so a hard `dev` dependency would be wrong. `dev-local` covers
  the single-machine case.
- URLs documented in the recipe comment. No Go changes. (Release-time `go:embed`+`exec`
  supervisor is out of scope here — ADR 0010.)

## Step 2 — Store: centralized display-tick selector — ✅ DONE

`client/src/utils/live-store.ts`

- Added `previewEnabled` + `offsetMs` (clamped 0–`MAX_OFFSET_MS`=2000), setters, persisted to
  localStorage (`forza.preview`, SSR-guarded). WHEP URL deferred to Step 4 per plan.
- `displayIndex(slice)` / `displayTick(slice)` pure helpers + `useDisplayTick()` reactive hook.
  Anchor is the ring's **own newest `sts`** (epoch-ns), not wall-clock → clock-skew-proof.
  Uses **floor** (newest tick at-or-before `newest.sts − offset`), clamps to oldest if the
  offset exceeds the buffered span.
- Swapped all four tick consumers, not just the two views:
  - `live.tsx` HUD → `useDisplayTick()` (reactive).
  - `InstrumentCanvas` (rAF) → `displayTick(getState())`.
  - `DynoCurve` (rAF, newest-only) → `displayTick(getState())`.
  - `Sparkline` (rAF, trailing window) → delays the window **end** via `displayIndex`; slices the
    ring only when delayed (off path still passes the ring uncopied — no per-frame allocation).
  Delaying the sparkline/dyno too keeps the *whole* dashboard on one coherent delayed instant, so
  nothing leads the speedo in preview mode.
- Self-check: `live-store.test.ts` — synthetic ring asserts off/zero/floor/exact/over-span/empty
  cases. `pnpm test` green (35 passed); `tsc --noEmit` clean.
- Note: `pnpm lint` is already red on `HEAD` repo-wide (`react-in-jsx-scope` misconfig + test
  type-assertion rule); not introduced here. New test matches the existing `as unknown as` idiom.

## Step 3 — WHEP video pane — ✅ DONE

`client/src/components/GamePreview.tsx` (new)

- Hand-rolled WHEP client, no new dep: recvonly `RTCPeerConnection` (video+audio) → non-trickle
  offer (waits for ICE gathering) → POST SDP to the WHEP URL → set answer → `video.srcObject`.
- `<video autoPlay muted playsInline object-contain>` on a black panel; mute/unmute button starts
  muted (autoplay policy).
- Status overlay: `idle` (no URL → "open preview settings") / `connecting` (spinner) / `playing`.
- Capped backoff retry 500 ms → 10 s (mirrors `LiveSocket`) on fetch error or ICE
  failed/disconnected/closed. Full teardown on unmount / URL change (`stopped` guard + `pc.close()`).
- Takes `url` as a prop; Step 4 supplies it from localStorage. `tsc` clean.

## Step 4 — Toggle, split layout, settings — ✅ DONE

`client/src/components/LivePreview.tsx` (new, shared) + both live routes.

- `PreviewToggle` (HeroUI `Button`, `monitor-play` icon) in both live headers → flips
  `previewEnabled`. `PreviewShell` wraps each view: off → children full-width (unchanged);
  on → resizable split `[ PreviewPane | dashboard ]`. Both HUD and Instrument share both.
- Added `whepUrl` to the store (persisted in the same `forza.preview` localStorage blob; not
  used by the selector, only the pane).
- Settings are an **inline bar** under the video (not a popover — no Slider/Popover component
  exists in the codebase, native styled inputs avoid HeroUI v3 API guessing): WHEP URL text
  field + offset `range` (0–`MAX_OFFSET_MS`, 10 ms step) with live ms readout. Both → localStorage.
- Glass tokens throughout (`bg-surface`, `bg-surface-secondary`, `text-muted`, `rounded-2xl`,
  `shadow-surface`); accent via `var(--accent)` (token ref, not raw color) — DESIGN.md.
- **Gotcha 1:** `react-resizable-panels@4` renamed exports — `PanelGroup`→`Group` (prop
  `direction`→`orientation`), `PanelResizeHandle`→`Separator`; `Panel` unchanged.
- **Gotcha 2:** in v4, **numeric** `defaultSize`/`minSize` = *pixels*; percentages must be
  **strings** (`"45%"`/`"25%"`). A bare `defaultSize={45}` rendered a 45px-wide panel. Caught
  only by browser inspection — the build was green.
- `tsc` clean, `pnpm test` green (35), `pnpm build` bundles (`LivePreview` 8.2 kB).
- **Browser smoke test (Chrome via devtools):** preview-on split renders both panes at full
  height with ~45%/55% widths and a 16:9 video; settings bar (URL + offset) reads/writes the
  store; preview-off renders full-width with no panels/video and no React key warning. Video
  status correctly sits at "Connecting…" against an unreachable stream (graceful retry). Not yet
  verified with a live stream + Go server — that's Step 5 calibration.

## Step 5 — Calibrate & document — ◐ docs done, calibration is manual

### Usage

1. **Relay:** run `just media` on a box both OBS and the viewer can reach (the game PC for the
   split topology; any box for all-on-one). Override the binary with `just media MEDIAMTX=…`.
2. **OBS (game PC):** Settings → Stream → Service **WHIP**, Server
   `http://<relay-lan-ip>:8889/mystream/whip`, blank bearer token → Start Streaming. Add the game
   as a source. Capture game **audio natively in OBS** for any recording (don't rely on the
   round-tripped in-UI audio).
3. **Dashboard:** open `/live` (or `/live/instrument`), click **Game preview**. In the settings
   bar set the WHEP URL to `http://<relay-lan-ip>:8889/mystream/whep`. The video pane connects.
4. **Calibrate (manual):** nudge the **Sync offset** slider until the speedo/wheels line up with
   the video. Persists across reloads (localStorage). Re-calibrate per device/network.

### Export (neither needs app code)

- **Same-PC, high quality:** in OBS, composite **native game capture + a browser source pointed at
  the dashboard in *normal* mode** (preview toggle off, zero-delay). The dashboard lags the game
  slightly (WS+render); correct it with OBS's **Video Delay (async)** filter on the game source.
  Record/stream the OBS scene.
- **Cross-machine:** screen-record the browser with the dashboard in **preview+delay mode** — the
  in-UI Sync offset is the sync, and the video pane is already composited beside the telemetry.

## Out of scope (deferred)

- `go:embed`+`exec` MediaMTX supervisor (release-time packaging).
- Historical stint video (storage, per-stint association, seek-sync) — a different feature.
- Server-side / shared config (promote WHEP URL out of localStorage only if multi-device re-entry annoys).
- Auto-sync (correlation / burned-in timecode).
