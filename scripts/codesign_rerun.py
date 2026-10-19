#!/usr/bin/env python3
"""Operator independent-rerun: spec + blank -> evaluator only, sealed receipt.

Usage (from repo root):
  platform/.venv/bin/python scripts/codesign_rerun.py --spec and2 --budget 8 --seed 7
  platform/.venv/bin/python scripts/codesign_rerun.py --spec and2 --ledger-path /tmp/rr.jsonl

Simulator-only, deterministic, no QPU, no LLM network. The candidate pool
is the built-in ladder (blank -> wrong -> valid); a paper-scale rerun
supplies its own proposer via the library, never via this script's flags.
Exit 0 with receipt JSON on stdout; exit 2 when no valid circuit found
(honest no-result, never a fake best).
"""

from __future__ import annotations

import argparse
import json
import sys

sys.path.insert(0, "platform")

from kubona.codesign import IndependentRerun, build_spec  # noqa: E402

CANDIDATE_LADDERS: dict[str, list] = {
    "and2": [[], [("cx", (0, 2), ())], [("ccx", (0, 1, 2), ())]],
    "xor2": [[], [("cx", (0, 2), ())], [("cx", (0, 2), ()), ("cx", (1, 2), ())]],
    "copy1": [[], [("x", (1,), ())], [("cx", (0, 1), ())]],
    "cz-phase": [[], [("z", (0,), ())], [("cz", (0, 1), ())]],
}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--spec", default="and2")
    ap.add_argument("--budget", type=int, default=8)
    ap.add_argument("--seed", type=int, default=7)
    ap.add_argument("--num-tests", type=int, default=64)
    ap.add_argument("--ledger-path", default=None)
    args = ap.parse_args()

    try:
        spec = build_spec(args.spec)
    except ValueError as e:
        print(json.dumps({"ok": False, "error": str(e)}))
        return 2
    ladder = CANDIDATE_LADDERS.get(args.spec)
    if ladder is None:
        print(json.dumps({"ok": False, "error": f"no rerun ladder for {args.spec!r}: refusing"}))
        return 2

    def _prop(_current: list, step: int) -> list:
        return list(ladder[step % len(ladder)])

    ledger = None
    if args.ledger_path:
        from kubona.provenance import Ledger

        ledger = Ledger(path=args.ledger_path)
    receipt = IndependentRerun(
        spec, args.spec, budget_steps=args.budget, seed=args.seed,
        num_tests=args.num_tests, proposer=_prop, ledger=ledger).run()
    print(json.dumps({"ok": True, "receipt": receipt}, default=str))
    return 0 if receipt["best"] is not None else 2


if __name__ == "__main__":
    sys.exit(main())
