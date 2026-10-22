# Contributing to Kubona

Thank you for your interest in contributing to **Kubona** — the open-source, MCP-native quantum-computing and AI-enhanced science discovery platform for autonomous AI agents.

Kubona is built to a high standard of engineering rigor, mathematical correctness, and strict licence discipline. Work is conducted on `develop`; `main` advances by fast-forward only.

---

## 📜 Licensing & Developer Certificate of Origin (DCO)

All contributions to this repository must be legally unencumbered and properly licensed. Kubona operates under a dual-layer licensing model:

1. **Software & Codebase**: All software contributions (Go, Python, TypeScript, Rust, shell scripts, configurations) are licensed under the **[Apache License, Version 2.0](LICENSE)**.
2. **Documentation & Blueprint Knowledge Base**: All documentation, architecture specifications, and living knowledge base pages (`wiki/**`) are licensed under **[Creative Commons Attribution 4.0 International (CC-BY-4.0)](https://creativecommons.org/licenses/by/4.0/)**.

### Developer Certificate of Origin (DCO 1.1)

To ensure intellectual property and provenance integrity, Kubona enforces the standard **Developer Certificate of Origin (DCO)**. By contributing, you certify that you have the right to submit the work under the project's licenses.

Every commit must include a `Signed-off-by:` line with your real name and email (conforming to `git commit -s`):

```text
Signed-off-by: Your Name <your.email@example.com>
```

#### DCO Text:
```text
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it; and

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

---

## 🛡️ Integration Topology & Licence Discipline

Kubona strictly separates dependencies into four explicit architectural classes before any third-party code or insight enters the platform (see `wiki/concepts/integration-topology-and-licence-discipline.md`):

1. **Submodule**: Pinned, compatible permissive licenses (Apache-2.0, MIT, BSD) retained with their upstream `LICENSE` and `NOTICE` files intact.
2. **External binary**: Invoked strictly as an isolated system process (e.g. LLVM/MLIR, provider CLIs); never vendored inside the core platform.
3. **Sidecar**: Services reached across process/network boundaries (HTTP/gRPC/MCP) with graceful degradation (e.g. OpenBao under MPL-2.0). Strong copyleft (GPL/AGPL) or weak copyleft code is **never** linked directly into the Apache-2.0 core.
4. **Blueprint**: Concepts read from academic literature or external repos; independently implemented and proven via behavioral tests under Kubona's Apache-2.0 license.

### Dataset & Benchmark Integrity
All benchmarks, evaluators, and RAG corpora must have verified per-item license metadata. Non-commercial (`CC-BY-NC`) or unlicensed scraped material will be refused by automated loaders to prevent downstream contamination of scientific findings.

---

## 🏛️ Production Bar & Engineering Standards

Kubona is built for production, not demonstration (see `AGENTS.md`):

- **Real Logic or Fail-Closed Refusal**: No mocked `success: true`, no arithmetic standing in for measurement, and no verification without empirical evidence or formal mathematical proof.
- **No Weakened Tests**: When updating a test for new behavior, assert the corrected behavior with equal or greater strength. Red going green by asserting less is prohibited.
- **Line Audit Before Landing**: Review the entire diff line by line before submitting: edge cases, error handling, floating-point precision, ordering subtleties, and backward compatibility.

---

## 🧪 Test Lanes (R0)

Run the cheapest lane that covers your change; CI runs `test:boundary` on every push to `main`/`develop` (see `.github/workflows/ci.yml`).

### test:fast — no installs, < 2 min (required before every commit)

```sh
python3 scripts/wiki-lint.py --strict
./scripts/secret-scan.sh
go build ./...                          # from platform/core
```

### test:changed — diff-scoped suites (required when touching code)

```sh
# Python: run the test files matching your changed modules, e.g.
platform/.venv/bin/pytest platform/tests/test_event_taxonomy.py -q
# Go: run the affected packages with -count=1 (never cached), e.g.
go test -count=1 ./pkg/ledger/ ./pkg/api/   # from platform/core
# Console: only when touching platform/console
pnpm --dir platform/console test --run <changed-spec>
```

Pattern: `git diff --name-only` → one suite per touched area. A green `test:changed` is necessary but not sufficient — it cannot catch cross-area breaks (that is what `test:boundary` is for).

### test:boundary — full CI mirror (required before push)

```sh
go test -race ./...                     # from platform/core
platform/.venv/bin/pytest platform/tests/ -q
KUBONA_STATE_DIR="$(mktemp -d)" platform/.venv/bin/python -m kubona.run doctor  # from platform
pnpm --dir platform/console test && pnpm --dir platform/console build
cargo test --lib -p kubona-qir-core     # from platform/crates/qir-core
```

Lane mapping to CI: `test:fast` ≈ `wiki-lint` + `secret-scan` jobs; `test:boundary` ≈ all jobs; the `canary-gate` job runs only when every suite job is green, so a red suite can never hide behind a green gate.

**Key Invariants:**
- Python version matrix: local dev may run 3.12, but CI pins 3.11 — keep `requires-python` and syntax compatible with the older interpreter.
- Go: always `-count=1`; cached passes have hidden real breaks.
- Never commit `dev-docs/` or `.obsidian/` (pre-commit guard enforces); never commit `.env`.

---

## 🚀 Submitting Contributions

1. **Fork the repository** and create a feature branch off `develop`.
2. **Write clean, documented, tested code** adhering to the conventions above.
3. **Sign your commits** with `git commit -s`.
4. **Ensure pre-commit checks and test lanes pass**.
5. **Open a Pull Request** against `develop` describing your changes, motivation, and test proof.
