import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/live")({
  component: LiveRoute,
});

// TODO: subscribe to `~/utils/ws` LiveSocket, render gauges from
// `~/utils/live-store` (speed, RPM, gear, lat/long G, throttle/brake).
function LiveRoute() {
  return (
    <div className="p-4">
      <h1 className="text-2xl font-bold">Live HUD</h1>
      <p className="text-sm text-gray-600">Placeholder — real-time WebSocket view lands here.</p>
    </div>
  );
}
