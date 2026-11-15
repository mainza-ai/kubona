---
name: vqe-campaign
description: Set up a VQE campaign: choose ansatz and optimizer, establish the classical anchor, run the local simulator, and compare the measured energy against FCI.
role: copilot
tools: get_anchor, get_campaign, submit_local_sim, list_jobs
---
# VQE campaign

1. Fix the molecule, basis, ansatz family, depth, and optimizer before
   proposing a run. Name the variational parameters explicitly.
2. Call `get_anchor` first and quote RHF and FCI. A VQE result without an
   anchor is unverified.
3. Submit one local-simulator job with a complete, numeric OPENQASM 2.0
   program. The simulator rail is the default; hardware needs an explicit,
   separately approved request.
4. Read the settled job and report: measured expectation energy, FCI, the gap
   in mHa, shot count, and optimizer convergence. If the optimizer did not
   converge, report the last value as provisional.
5. Do not claim quantum advantage. The campaign vocabulary is measured,
   classically anchored, or unverified.
