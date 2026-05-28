/**
 * TanStack Query `queryOptions` factories for the REST v1 endpoints.
 *
 * Each factory returns a `queryOptions(...)` object ready for `useQuery`,
 * `prefetchQuery`, route loaders, and `useSuspenseQuery`. Keys are tuple-
 * shaped so React Query's selectQueries/invalidate patterns work cleanly.
 */
import { queryOptions } from "@tanstack/react-query";
import { fetchAndParse } from "~/utils/api";
import {
  HotSpotsResponseSchema,
  LapsResponseSchema,
  PathResponseSchema,
  PreviewResponseSchema,
  SessionDetailSchema,
  SessionsListResponseSchema,
  StintDetailSchema,
  StraightsResponseSchema,
  TurnsResponseSchema,
} from "~/utils/schemas";

// --- Sessions ---

export const sessionsListQuery = () =>
  queryOptions({
    queryKey: ["sessions"] as const,
    queryFn: () => fetchAndParse("/sessions", SessionsListResponseSchema),
  });

export const sessionQuery = (id: string) =>
  queryOptions({
    queryKey: ["sessions", id] as const,
    queryFn: () => fetchAndParse(`/sessions/${id}`, SessionDetailSchema),
  });

// --- Stints + sub-resources ---

export const stintQuery = (id: string) =>
  queryOptions({
    queryKey: ["stints", id] as const,
    queryFn: () => fetchAndParse(`/stints/${id}`, StintDetailSchema),
  });

export const lapsQuery = (id: string) =>
  queryOptions({
    queryKey: ["stints", id, "laps"] as const,
    queryFn: () => fetchAndParse(`/stints/${id}/laps`, LapsResponseSchema),
  });

export const hotSpotsQuery = (id: string) =>
  queryOptions({
    queryKey: ["stints", id, "hot-spots"] as const,
    queryFn: () => fetchAndParse(`/stints/${id}/hot-spots`, HotSpotsResponseSchema),
  });

export const turnsQuery = (id: string) =>
  queryOptions({
    queryKey: ["stints", id, "turns"] as const,
    queryFn: () => fetchAndParse(`/stints/${id}/turns`, TurnsResponseSchema),
  });

export const straightsQuery = (id: string) =>
  queryOptions({
    queryKey: ["stints", id, "straights"] as const,
    queryFn: () => fetchAndParse(`/stints/${id}/straights`, StraightsResponseSchema),
  });

export const previewQuery = (id: string) =>
  queryOptions({
    queryKey: ["stints", id, "preview"] as const,
    queryFn: () => fetchAndParse(`/stints/${id}/preview`, PreviewResponseSchema),
  });

// Downsampled 3D path for the track-path minimap. `step` defaults to 6 on the
// server (~10Hz from 60Hz parquet) — enough density for smooth curves without
// the per-stint payload weight of full-rate ticks.
export const pathQuery = (id: string, step?: number) =>
  queryOptions({
    queryKey: ["stints", id, "path", step ?? "default"] as const,
    queryFn: () => {
      const qs = step !== undefined ? `?step=${step}` : "";
      return fetchAndParse(`/stints/${id}/path${qs}`, PathResponseSchema);
    },
  });
