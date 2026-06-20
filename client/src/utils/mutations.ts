/**
 * TanStack Query mutations for the REST v1 write endpoints.
 *
 * Pin toggles `PATCH /sessions/{id}` and invalidates the `["sessions"]` key
 * prefix (which covers both the list and the `["sessions", id]` detail). The
 * downsample action posts to a stubbed endpoint that returns 501 today — the
 * error branch surfaces that politely rather than as a hard failure.
 */
import { toast } from "@heroui/react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "~/utils/api";

export function useSessionPin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, pinned }: { id: string; pinned: boolean }) =>
      apiFetch(`/sessions/${id}`, { method: "PATCH", body: { pinned } }),
    onError: () => toast.danger("Couldn't update pin"),
    onSuccess: (_data, { pinned }) => {
      void qc.invalidateQueries({ queryKey: ["sessions"] });
      toast.success(pinned ? "Session pinned" : "Session unpinned");
    },
  });
}

export function useSessionDownsample() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiFetch(`/sessions/${id}/downsample`, { method: "POST" }),
    // The endpoint is a stable 501 stub until the Parquet-rewrite job lands.
    onError: () =>
      toast.info("Downsampling isn't available yet", {
        description: "The backend job lands in a later update.",
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["sessions"] });
      toast.success("Session downsampled");
    },
  });
}

export function useSessionDelete() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiFetch(`/sessions/${id}`, { method: "DELETE" }),
    onError: () => toast.danger("Couldn't delete session"),
    onSuccess: (_data, id) => {
      // Drop the detail cache (the resource is gone — don't let an invalidate
      // refetch it into a 404), refresh the list, and clear stint pages that
      // may have belonged to this session (broad but safe for a single user).
      qc.removeQueries({ queryKey: ["sessions", id] });
      void qc.invalidateQueries({ queryKey: ["sessions"], exact: true });
      qc.removeQueries({ queryKey: ["stints"] });
      toast.success("Session deleted");
    },
  });
}

export function useStintDelete() {
  const qc = useQueryClient();
  return useMutation({
    // sessionId rides along only so callers can navigate to the parent.
    mutationFn: ({ id }: { id: string; sessionId: string }) =>
      apiFetch(`/stints/${id}`, { method: "DELETE" }),
    onError: () => toast.danger("Couldn't delete stint"),
    onSuccess: (_data, { id }) => {
      // Remove the stint detail + its sub-resource caches (prefix match), and
      // refresh sessions so the parent's stint list + counts update.
      qc.removeQueries({ queryKey: ["stints", id] });
      void qc.invalidateQueries({ queryKey: ["sessions"] });
      toast.success("Stint deleted");
    },
  });
}
