import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/")({
  component: HomeRoute,
});

function HomeRoute() {
  return (
    <div className="p-4">
      <h1 className="text-2xl font-bold">Forza Telemetry</h1>
      <p className="text-sm text-gray-600">
        Pick <span className="font-mono">Live</span> for the realtime HUD, or{" "}
        <span className="font-mono">Sessions</span> to browse captured stints.
      </p>
    </div>
  );
}
