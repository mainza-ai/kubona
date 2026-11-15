---
name: classical-anchor-check
description: Verify a claimed quantum result against a classical anchor before accepting or reporting it; name the exact anchor and the discrepancy.
role: copilot
tools: get_anchor, get_campaign
---
# Classical anchor check

Use this whenever a reply would report an energy, gap, or advantage.

1. Call `get_anchor` with the molecule and bond distance behind the claim.
2. Record both the RHF and FCI values that came back. If the anchor call
   failed, the claim is unverified; say so.
3. Compare the claimed value to FCI in mHa. State the tolerance and whether
   the claim is inside it.
4. If the claim came from a job, call `get_campaign` and confirm the job and
   measurement are settled, not queued or failed.
5. Never substitute a literature number for a live anchor unless the user
   explicitly asks for a literature comparison, and label it as such.

An unanchored number is never "close enough"; it is unverified.
