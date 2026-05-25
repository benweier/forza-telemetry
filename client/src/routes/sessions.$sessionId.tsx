import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/sessions/$sessionId")({
  component: SessionDetailRoute,
});

// TODO: useQuery against GET /api/v1/sessions/{id}; render session metadata +
// stint list with links to /stints/{stintId}.
function SessionDetailRoute() {
  const { sessionId } = Route.useParams();
  return (
    <div className="p-4">
      <h1 className="text-2xl font-bold">Session {sessionId}</h1>
      <p className="text-sm text-gray-600">Placeholder — session detail + stint list lands here.</p>
    </div>
  );
}
