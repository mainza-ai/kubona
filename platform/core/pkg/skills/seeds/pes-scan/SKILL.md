---
name: pes-scan
description: Plan a potential-energy-surface scan: fix the coordinate grid, establish the classical anchor first, then run the simulator and report the discrepancy in mHa.
role: copilot
tools: get_anchor, get_campaign, submit_local_sim, list_jobs
---
# Potential-energy-surface scan

1. State the molecule, basis, bond-distance range, and number of points.
2. Call `get_anchor` for the shortest and longest separations before proposing
   any quantum run. The anchor is the comparison, not an optional extra.
3. Propose one local-simulator job for the scan (one dispatch per turn). Use a
   complete OPENQASM 2.0 program with numeric gate arguments.
4. After settlement, call `get_job` / `get_campaign` and report each point's
   measured energy, the anchor value, and the discrepancy in mHa.
5. If the scan is incomplete or a point failed, say which point is missing and
   what would establish it. Never interpolate a missing measurement.

Refuse to claim a barrier height or minimum that the settled points do not
support.
