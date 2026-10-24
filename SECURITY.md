# Security Policy

## Reporting a vulnerability

**Do not open a public issue, discussion, or PR for a suspected
vulnerability.** Use GitHub's **private vulnerability reporting**:

1. Open the repository's **Security** tab.
2. Click **Report a vulnerability** (private advisory — visible only to
   you and the maintainers).

We aim to acknowledge within **5 business days** and to ship a fix or a
mitigation within **30 days** for high-severity issues. If the reporting
button is unavailable, treat that as a bug in this file and tell us
through any private channel you have — do not post details publicly.

## Safe harbor

Good-faith research against this policy is welcomed. Do not access other
operators' data, do not degrade shared services, and stop and report if
you gain access you should not have. We will not pursue legal action for
conduct that stays within these bounds.

## Scope

Kubona orchestrates AI agents against quantum-hardware spend rails and
federated credentials, so the trust root is the governance layer itself.
The following classes are **in scope** — several are design features an
outsider could mistake for intended behavior, so they are named
explicitly:

- **Prompt-injection → spend execution.** Model text (including DSML
  markup, tool-call blocks, approval packets, or world-tool arguments)
  that causes unauthorized tool execution, mandate minting, or spend.
- **Ledger bypass or receipt forgery.** Anything that seals, appends,
  replays, or resolves ledger records — or mints/binds spend mandates —
  without passing the authenticated + admin-envelope path.
  (`/api/ledger/append` requires the operator bearer; mandate spend
  additionally requires the tenant admin's Ed25519 envelope.)
- **Spending-authority confusion.** Session/lab attribution spoofing,
  idempotency-key games causing double spend, or cap/reservation bypasses
  in the session, tenant, or loop rails.
- **Credential exposure or escalation.** Leaked operator tokens, session
  fixation or cookie theft, CORS/origin confusion that upgrades a read
  into a write, or cloud-provider keys reachable where they should not be.
- **Dependency and supply-chain compromise.** Malicious or vulnerable
  pinned dependencies, submodule swaps, or build-pipeline tampering.

Out of scope: denial-of-service volume testing, social engineering of
contributors, and reports about missing hardening already tracked as open
work in `wiki/` (check `wiki/questions.md` first — duplicates of known
items will be closed as such, still gratefully).

## What to include

- The exact route, tool, or ledger event type, with a minimal
  reproduction (curl, test, or transcript).
- What you expected the governance layer to refuse, and what happened.
- Which credentials or roles you used (anonymous, bearer, lab admin —
  never send us a live secret; redact tokens and keys).
- Impact assessment: what an attacker could spend, forge, or access.

## Supported versions

Only the tips of `main` and `develop` are supported. There are no
stable release lines yet; advisories are patched forward.

## Operational notes (for maintainers)

- GitHub-native controls are on: secret scanning, push protection, and
  Dependabot security updates (see repository settings); CI mirrors the
  secret scan in `scripts/secret-scan.sh` and enforces DCO sign-off.
- Credentials live in the environment or the OpenBao sidecar, never in
  the tree (`.env`, `dev-docs/`, `.obsidian/` are gitignored and CI-gated).
- `NEXT_PUBLIC_*` values ship in the client JS bundle by construction:
  never put an operator token there in production (the console warns at
  runtime if one is present in a production build).
