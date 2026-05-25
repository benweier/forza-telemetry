import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/stints/$stintId")({
  component: StintDetailRoute,
});

// TODO: useQuery for GET /api/v1/stints/{id} (detail + summary), then sub-
// resources: laps, hot-spots, corners, preview, and on-demand tick windows.
// Render: time-series chart, mini-map, hot-spot pins, lap table.
function StintDetailRoute() {
  const { stintId } = Route.useParams();
  return (
    <div className="p-4">
      <h1 className="text-2xl font-bold">Stint {stintId}</h1>
      <p className="text-sm text-gray-600">Placeholder — stint charts + hot-spots + corners land here.</p>
    </div>
  );
}
