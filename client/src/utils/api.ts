import { up } from "up-fetch";
import * as v from "valibot";

/**
 * REST client for the Go telemetry server's v1 API. Base URL is relative
 * so the same client works behind the Go SPA embed (single origin in prod)
 * and the Vite dev proxy on :3000 (which forwards /api → :8080).
 *
 * up-fetch throws on non-2xx by default — caller catches via Promise rejection
 * (React Query renders as an `error` state).
 */
export const apiFetch = up(fetch, () => ({
  baseUrl: "/api/v1",
}));

/**
 * Helper: fetch + validate. Wraps apiFetch with a schema and returns the
 * parsed typed result. Use in queryOptions queryFn.
 */
export async function fetchAndParse<TSchema extends v.GenericSchema>(
  path: string,
  schema: TSchema,
): Promise<v.InferOutput<TSchema>> {
  const raw = await apiFetch(path);
  return v.parse(schema, raw);
}
