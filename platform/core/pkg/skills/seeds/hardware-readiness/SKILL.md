---
name: hardware-readiness
description: Check hardware readiness before any QPU spend: credentials, backend status, budget, calibration freshness, and a live classical anchor.
role: copilot
tools: get_campaign, list_jobs, get_anchor
---
# Hardware readiness

Hardware dispatch is not a tool; it is a proposal followed by the approval
gate. Use this skill to tell the user whether a run is ready to propose.

1. Call `get_campaign` for the session budget, backend preference, and any
   prior hardware attempt.
2. Call `list_jobs` and identify queued, running, failed, or settled hardware
   jobs. Never describe a queued job as a result.
3. Call `get_anchor` for the target system. Without an anchor, a hardware run
   cannot be judged afterward.
4. State each gate separately: credential present, backend reachable, budget
   remaining, calibration acceptable, anchor present. Unknown is not met.
5. If any gate is unknown or unmet, say which one and what would establish it.
   Do not estimate a cost or a queue time that the server did not return.
