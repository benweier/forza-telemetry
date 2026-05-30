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
