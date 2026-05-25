import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/sessions/")({
  component: SessionsIndexRoute,
});

// TODO: useQuery against GET /api/v1/sessions; render list with stint counts,
// pin toggle, and link to /sessions/{id}.
function SessionsIndexRoute() {
  return (
    <div className="p-4">
      <h1 className="text-2xl font-bold">Sessions</h1>
      <p className="text-sm text-gray-600">Placeholder — sessions list lands here.</p>
    </div>
  );
}
