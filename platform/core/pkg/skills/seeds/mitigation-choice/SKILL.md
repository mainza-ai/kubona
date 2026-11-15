---
name: mitigation-choice
description: Choose an error-mitigation strategy from the measured noise and circuit shape; state the assumption and expected cost before recommending one.
role: copilot
tools: get_campaign, get_job, recent_events
---
# Mitigation choice

1. Establish the circuit shape and depth, the backend or simulator noise model,
   and the observable being estimated. Without those, the choice is a guess.
2. Call `get_campaign` / `get_job` / `recent_events` for measured noise,
   fidelity, and any prior mitigation attempt in this session.
3. Prefer the least expensive method that addresses the observed error:
   readout calibration for measurement bias, zero-noise extrapolation for
   coherent gate error, and probabilistic error cancellation only when the
   noise model is good enough to sample it.
4. State the assumption, the extra shots or circuits, and the cost class
   before recommending a run. Never promise an improvement you did not measure.
5. If the evidence is insufficient, say which measurement would decide the
   choice instead of picking one arbitrarily.
