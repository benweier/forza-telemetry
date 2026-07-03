/* Hallmark · component: game-preview · genre: dashboard · theme: Glass */
import { Icon } from "@iconify/react";
import { useEffect, useRef, useState } from "react";

/** Resolve once ICE gathering finishes so we can POST a complete (non-trickle)
 *  SDP offer — MediaMTX's WHEP endpoint expects the full candidate set. */
function waitIceGatheringComplete(pc: RTCPeerConnection): Promise<void> {
  if (pc.iceGatheringState === "complete") return Promise.resolve();
  return new Promise((resolve) => {
    const check = () => {
      if (pc.iceGatheringState === "complete") {
        pc.removeEventListener("icegatheringstatechange", check);
        resolve();
      }
    };
    pc.addEventListener("icegatheringstatechange", check);
  });
}

type Status = "idle" | "connecting" | "playing";

/**
 * Plays a WebRTC game stream via WHEP into a <video>. Recvonly RTCPeerConnection,
 * non-trickle offer POSTed to the WHEP URL, answer applied (see ADR 0010 / the
 * spike at spike/whep.html). Reconnects with capped backoff like LiveSocket so a
 * stopped OBS or a restarted relay recovers on its own. The Go server is never
 * involved — the URL points straight at MediaMTX.
 */
export function GamePreview({ url }: { url: string }) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [status, setStatus] = useState<Status>("idle");
  // Autoplay policy: must start muted. The user unmutes explicitly.
  const [muted, setMuted] = useState(true);

  // React sets `muted` as a property unreliably across renders; enforce it on the
  // element so autoplay isn't blocked and unmute takes effect immediately.
  useEffect(() => {
    if (videoRef.current) videoRef.current.muted = muted;
  }, [muted]);

  useEffect(() => {
    if (!url) {
      setStatus("idle");
      return undefined;
    }

    let stopped = false;
    let pc: RTCPeerConnection | null = null;
    let retryMs = 500;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;

    const teardown = () => {
      if (pc) {
        pc.close();
        pc = null;
      }
    };

    const scheduleRetry = () => {
      if (stopped) return;
      teardown();
      setStatus("connecting");
      // One failure can reach here twice (ICE state handler + connect's catch);
      // without this clear, two timers stack and connect() runs concurrently,
      // leaking the overwritten RTCPeerConnection.
      clearTimeout(retryTimer);
      retryTimer = setTimeout(() => void connect(), retryMs);
      retryMs = Math.min(retryMs * 2, 10_000);
    };

    const connect = async () => {
      if (stopped) return;
      setStatus("connecting");
      try {
        pc = new RTCPeerConnection();
        pc.addTransceiver("video", { direction: "recvonly" });
        pc.addTransceiver("audio", { direction: "recvonly" });
        pc.ontrack = (e) => {
          if (videoRef.current) videoRef.current.srcObject = e.streams[0];
        };
        pc.oniceconnectionstatechange = () => {
          if (!pc) return;
          const s = pc.iceConnectionState;
          if (s === "connected" || s === "completed") {
            retryMs = 500;
            setStatus("playing");
          } else if (s === "failed" || s === "disconnected" || s === "closed") {
            scheduleRetry();
          }
        };

        await pc.setLocalDescription(await pc.createOffer());
        await waitIceGatheringComplete(pc);
        if (stopped || !pc.localDescription) return;

        const res = await fetch(url, {
          method: "POST",
          headers: { "Content-Type": "application/sdp" },
          body: pc.localDescription.sdp,
        });
        if (!res.ok) throw new Error(`WHEP HTTP ${res.status}`);
        const answer = await res.text();
        if (stopped) return;
        await pc.setRemoteDescription({ type: "answer", sdp: answer });
      } catch {
        scheduleRetry();
      }
    };

    void connect();
    return () => {
      stopped = true;
      clearTimeout(retryTimer);
      teardown();
    };
  }, [url]);

  return (
    <div className="relative aspect-video w-full overflow-hidden rounded-2xl bg-black shadow-surface">
      <video
        ref={videoRef}
        autoPlay
        muted={muted}
        playsInline
        aria-label="Live game preview stream"
        onPlaying={() => setStatus("playing")}
        className="size-full object-contain"
      />

      {status !== "playing" && (
        <div className="absolute inset-0 grid place-items-center bg-surface/80 text-center">
          <div className="flex max-w-xs flex-col items-center gap-2 px-6">
            <Icon
              icon={url ? "lucide:loader-circle" : "lucide:monitor-play"}
              className={`size-7 text-muted ${status === "connecting" ? "animate-spin" : ""}`}
            />
            <p className="text-sm text-pretty text-muted">
              {url ? "Connecting to game stream…" : "No stream URL set — open preview settings."}
            </p>
          </div>
        </div>
      )}

      {status === "playing" && (
        <button
          type="button"
          onClick={() => setMuted((m) => !m)}
          aria-label={muted ? "Unmute" : "Mute"}
          className="absolute right-3 bottom-3 grid size-9 place-items-center rounded-xl bg-surface/80 text-foreground shadow-surface backdrop-blur"
        >
          <Icon icon={muted ? "lucide:volume-off" : "lucide:volume-2"} className="size-4" />
        </button>
      )}
    </div>
  );
}
