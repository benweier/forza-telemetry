# Corner detection blends path curvature with lateral G

**Corners** are detected at ingest by combining geometric path curvature (κ = |dθ/ds|, computed from the **Track Path** resampled at uniform distance) with sustained lateral G. Curvature anchors corner identity (geometry is independent of driver speed and aggression — the same corner has the same κ signature on every Lap). Lateral G confirms the corner was driven as one. Corner boundaries are extended backwards into the braking zone and forwards into the exit acceleration zone using longitudinal G thresholds.

## Consequences

- Corner numbering is stable across Laps over the same track, even when one Lap clips an apex and another runs wide.
- Comparison view can render "Lap A Corner 3 vs Lap B Corner 3" with proper alignment.
- Two ingest-time signal-processing passes (curvature on path; lateral G + longitudinal G on time-series) feed a single Corner extraction step.
- Requires resampling the Track Path at uniform distance intervals (e.g. every 1 m) as a pre-step.
