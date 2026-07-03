import { Unpackr } from "msgpackr";
import { useLiveStore } from "~/utils/live-store";
import type { TickFrame } from "~/types/tick.generated";

const ENV_HELLO = 1;
const ENV_TICK = 2;

// int64AsNumber: epoch-ns timestamps (sts) exceed Number.MAX_SAFE_INTEGER, which
// msgpackr decodes as BigInt by default — mixing BigInt with number arithmetic in
// consumers throws. The generated TickFrame types declare these fields as `number`;
// decode to number to honour that contract (sub-µs precision loss is irrelevant here).
const unpackr = new Unpackr({ useRecords: false, int64AsNumber: true });

export class LiveSocket {
  private ws: WebSocket | null = null;
  private path: string;
  private stopped = false;
  private retryMS = 500;

  constructor(path: string) {
    this.path = path;
  }

  start() {
    this.stopped = false;
    this.connect();
  }

  stop() {
    this.stopped = true;
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  private connect() {
    if (this.stopped) return;
    const url = wsUrl(this.path);
    const ws = new WebSocket(url);
    ws.binaryType = "arraybuffer";

    ws.addEventListener("open", () => {
      this.retryMS = 500;
      useLiveStore.getState().setConnected(true);
    });

    ws.addEventListener("close", () => {
      useLiveStore.getState().setConnected(false);
      this.ws = null;
      if (this.stopped) return;
      setTimeout(() => this.connect(), this.retryMS);
      this.retryMS = Math.min(this.retryMS * 2, 10_000);
    });

    ws.addEventListener("error", () => {
      ws.close();
    });

    ws.addEventListener("message", (ev) => {
      if (!(ev.data instanceof ArrayBuffer)) return;
      const decoded: unknown = unpackr.unpack(new Uint8Array(ev.data));
      if (!isRecord(decoded)) return;
      if (decoded["k"] === ENV_TICK) {
        const tick = decoded["t"];
        if (isTickFrame(tick)) useLiveStore.getState().push(tick);
      } else if (decoded["k"] === ENV_HELLO) {
        // Future: react to server hello (protocol version, ring replay size).
      }
    });

    this.ws = ws;
  }
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null;
}

// Spot-check, not full validation: frames come from our own server, built from
// the same generated schema (tick.generated.ts). This guards against envelope
// shape drift — an old server or a foreign payload — not per-field drift.
function isTickFrame(v: unknown): v is TickFrame {
  return isRecord(v) && typeof v["sts"] === "number" && typeof v["gv"] === "number" && typeof v["sp"] === "number";
}

function wsUrl(path: string): string {
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}${path}`;
}
