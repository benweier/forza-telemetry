import { cylinderLabel, drivetrainLabel } from "./engine";
// client/src/components/hud/EngineBadge.tsx
/* Hallmark · component: engine-badge · genre: dashboard · theme: Glass */
import type { TickFrame } from "~/types/tick.generated";

/** Compact identity pill: cylinder count · drivetrain. Renders nothing useful
 *  on FH5/unknown packets where both fields are 0. */
export function EngineBadge({ tick }: { tick: TickFrame }) {
  const parts = [cylinderLabel(tick.ncy), drivetrainLabel(tick.dt)].filter(
    (p): p is string => p !== null,
  );
  if (parts.length === 0) return null;
  return (
    <span className="rounded-full border border-separator bg-surface-secondary px-3 py-1 text-xs font-medium text-foreground">
      {parts.join(" · ")}
    </span>
  );
}
