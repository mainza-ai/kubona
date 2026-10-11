---
type: concept
name: Secret Rotation Runbook
date: 2026-10-11
updated: 2026-10-11
tags: [security, secrets, operations, rotation]
sources: [source-gap-analysis-plan]
status: adopted
---

# Secret Rotation Runbook

Procedure only — **no secret values live on this page, in the wiki, or in
the repo, ever.** Each entry: where the secret is configured, what breaks
if it leaks, how to rotate, how to verify. Rotate in the order listed
(spend authority first).

## Principles

- Secrets are operator config (environment), never code. The tree has
  contained hardcoded secrets before (lease HMAC, PG password); tests pin
  their absence — `TestVaultLeaseRequiresKey`, `DefaultPostgresStoreConfig`
  with empty default.
- Every gate fails closed on missing config (503/403), never open.
- Rotation = generate → configure → verify → revoke the old value. Never
  reuse a value across secrets.
- `.env` is gitignored and pre-commit-guarded alongside `dev-docs/`, but
  hooks live in `.git/hooks/` (unversioned, lost on re-clone). Workstation
  discipline, not just tooling: keep `.env` out of backups, screenshots,
  and dotfile repos.

## Inventory

| Secret | Configured | Blast radius if leaked | Rotate | Verify |
|---|---|---|---|---|
| Reviewer private key | `KUBONA_REVIEWER_PRIVKEY_HEX` (daemon env) | Mints spend mandates via `/api/mandates/sign` (needs admin envelope + bearer too) | Generate new Ed25519 keypair; set env; restart daemon | `/api/mandates/sign` with old-key-era mandate fails verify; new mints verify |
| Operator API token | `KUBONA_API_TOKEN` (daemon env) + console `Authorization` header | Calls `/api/mandates/sign` (still needs admin envelope) | Set new value both sides; restart daemon | Sign without header → 401; wrong value → 401 |
| Lease HMAC key | `KUBONA_LEASE_HMAC_KEY` (daemon env, min 32 chars) | Forges vault lease receipts | Set new value; restart daemon; old signatures fail comparison | Unset/short → 503 on mint |
| Postgres DSN | `KUBONA_PG_DSN` (consumer env) | Full memory-store read/write | Rotate in Postgres; update env; restart consumers | Empty default refuses connection (no silent fallback) |
| Tenant admin keys | Per-tenant `admin_pubkey_hex` via `/api/tenants/admin-key` (current-admin-signed rotation) | Authorizes spend + ceiling + autonomy per tenant | Current admin signs rotation to new pubkey | Old-key envelope → 403 |
| OpenCode/cloud key | `.env` `OPENCODE_API_KEY` (never committed) | LLM spend on operator's account | Regenerate at provider; update `.env`; restart daemon | Funded-model completion succeeds; 402 surfaces honestly |
| Console token | `NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN` | Client-visible: NOT a secret — never accepted as API auth | Align value for UX only; real auth is `KUBONA_API_TOKEN` | — |
| Git credential helper | `~/.git-credentials` (see [[risks]]) | Repo push as operator | Revoke stale grant; drop `store` helper | Helper absent from gitconfig |

## Notes

- The console's `NEXT_PUBLIC_*` values ship to browsers. Treat every one
  as public by construction; the server's bearer gate documents this
  (`pkg/api/edge_auth.go`).
- Lease signatures are issuance receipts; no verifier consumes them yet —
  do not mistake rotation of the HMAC key for access revocation.
- Tenant admin rotation is first-write bootstrap then signed rotation;
  a tenant with no admin key cannot sign (403), by design.
