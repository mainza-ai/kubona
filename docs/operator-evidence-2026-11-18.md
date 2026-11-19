# Operator evidence register — 2026-11-18

These are **not implementation tasks**. No code slice can produce them, and the
repo must not claim them complete without dated external evidence. They block
only the production claims that depend on them.

| Item | Status | Evidence / next action |
|---|---|---|
| Manual VoiceOver/NVDA transcript | **Not claimed** | Automated axe/keyboard/reduced-motion gates are green. A dated walkthrough of every modal/surface (with issues filed/fixed) is still required. |
| `gho_…` GitHub token revocation | **Not claimed** | Confirm the provider response and record it; the repo cannot prove an external revocation. |
| Reviewer key on the deployment host | **Confirmed 2026-11-18** | `KUBONA_REVIEWER_PRIVKEY_HEX` present in the running `kubonad` (`reviewer_key_present=true`). |
| Paid-QPU invoice / instance / run id | **Not claimed** | The attached `ibm_marrakesh` evidence is Open Plan/free (`$0.00`); a paid invoice or paid-instance run id must be attached, or the claim marked unpaid. |
| Runtime rate card | **Not claimed** | Operator pricing decision due before 2026-11-29; the local-private runtime card currently defaults to the reviewed list-price card. |
| Fresh-volume reset (`local_private`) | **Not claimed** | Operator procedure on a verified snapshot; restores the declared posture. |

The code plan (`docs/active-implementation-todos.md`) is closed when no
code-addressable item remains. This register is the evidence ledger for the
claims the code cannot make.
