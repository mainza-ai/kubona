# Contributing — test lanes (R0)

Run the cheapest lane that covers your change; CI runs `test:boundary`
on every push to `main`/`develop` (see `.github/workflows/ci.yml`).
Work on `develop`; `main` advances by fast-forward only.

## test:fast — no installs, < 2 min (required before every commit)

```sh
python3 scripts/wiki-lint.py --strict
./scripts/secret-scan.sh
go build ./...                          # from platform/core
```

## test:changed — diff-scoped suites (required when touching code)

```sh
# Python: run the test files matching your changed modules, e.g.
platform/.venv/bin/pytest platform/tests/test_event_taxonomy.py -q
# Go: run the affected packages with -count=1 (never cached), e.g.
go test -count=1 ./pkg/ledger/ ./pkg/api/   # from platform/core
# Console: only when touching platform/console
pnpm --dir platform/console test --run <changed-spec>
```

Pattern: `git diff --name-only` → one suite per touched area. A green
`test:changed` is necessary but not sufficient — it cannot catch
cross-area breaks (that is what `test:boundary` is for).

## test:boundary — full CI mirror (required before push)

```sh
go test -race ./...                     # from platform/core
platform/.venv/bin/pytest platform/tests/ -q
KUBONA_STATE_DIR="$(mktemp -d)" platform/.venv/bin/python -m kubona.run doctor  # from platform
pnpm --dir platform/console test && pnpm --dir platform/console build
cargo test --lib -p kubona-qir-core     # from platform/crates/qir-core
```

Lane mapping to CI: `test:fast` ≈ `wiki-lint` + `secret-scan` jobs;
`test:boundary` ≈ all jobs; the `canary-gate` job runs only when every
suite job is green, so a red suite can never hide behind a green gate.

Notes:

- Python version matrix: local dev may run 3.12, but CI pins 3.11 —
  keep `requires-python` and syntax compatible with the older interpreter.
- Go: always `-count=1`; cached passes have hidden real breaks.
- Never commit `dev-docs/` or `.obsidian/` (pre-commit guard enforces);
  never commit `.env`.
