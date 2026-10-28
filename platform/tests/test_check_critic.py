"""Check-discrimination critic: every invariant check must reject breaking
mutants and accept preserving ones (mutation testing for the corpus).

The expectation table below is theory-first (entanglement removal breaks
Bell/GHZ; X-flips break basis outcomes; phases/permutations preserve) and
was confirmed by execution before locking. The wstate drop_first_twoq case
is the proof this earns its keep: removing the entangler still yields
Hamming-weight-1 states, so the check correctly accepts — the critic
distinguished preserving from breaking where a naive table would not.
"""

from kubona.eval import check_critic as cc
from kubona.eval.corpus_loader import _CHECKS, load_corpus_dir
from kubona.eval.corpora import TASKS

# task -> {mutant: expected check outcome}. False = breaking (must reject),
# True = preserving (must accept).
EXPECTATIONS: dict[str, dict[str, bool]] = {
    "katas_bell_phi_plus": {"drop_first_twoq": False, "append_x0": False,
                            "h_to_x": False},
    "katas_phi_minus": {"insert_z0": True},
    "katas_psi_plus": {"drop_first_twoq": False},
    "katas_ghz_4q": {"drop_first_twoq": False},
    "ghz5": {"drop_first_twoq": False},
    "katas_wstate_3q": {"append_x0": False, "drop_first_twoq": True},
    "katas_uniform_superposition_3q": {"drop_all_h": False,
                                      "append_x0": True},
    "katas_plus_state": {"append_x0": True},
    "katas_grover_oracle_11": {"append_x0": False},
    "bell_pairs_4q": {"drop_first_twoq": False},
}


def _load():
    import kubona.eval.corpora as corpora
    from pathlib import Path
    load_corpus_dir(Path(corpora.__file__).parent / "corpora")
    return corpora.TASKS


def test_reference_tasks_pass_own_checks():
    tasks = _load()
    for name in EXPECTATIONS:
        assert name in tasks, f"corpus task {name} missing"
        got = cc.run_mutant(tasks[name]["qasm"], tasks[name]["check"])
        assert got is True, f"{name}: reference must pass its own check"


def test_breaking_mutants_rejected_and_preserving_accepted():
    tasks = _load()
    failures = []
    for name, expected in EXPECTATIONS.items():
        report = cc.criticize_task(name, tasks[name]["qasm"],
                                   tasks[name]["check"], expected)
        for mutant, verdict in report["verdicts"].items():
            if not verdict.get("applicable", False):
                failures.append(f"{name}/{mutant}: mutant inapplicable")
            elif not verdict["ok"]:
                failures.append(
                    f"{name}/{mutant}: expected {verdict['expected']}, "
                    f"check said {verdict['got']}")
    assert not failures, failures


def test_every_check_kind_has_breaking_coverage():
    import json
    from pathlib import Path
    import kubona.eval.corpora as corpora

    tasks = _load()
    kinds: dict[str, str] = {}
    corpus_dir = Path(corpora.__file__).parent / "corpora"
    for f in corpus_dir.glob("*.json"):
        data = json.loads(f.read_text())
        for name, spec in data.items():
            if not name.startswith("_") and isinstance(spec, dict) and "check" in spec:
                kinds[name] = spec["check"]
    covered: dict[str, bool] = {kind: False for kind in _CHECKS}
    for name, expected in EXPECTATIONS.items():
        breaking = [m for m, exp in expected.items() if exp is False]
        if not breaking:
            continue
        report = cc.criticize_task(name, tasks[name]["qasm"],
                                   tasks[name]["check"],
                                   {m: False for m in breaking})
        if report["ok"]:
            covered[kinds[name]] = True
    missing = [k for k, v in covered.items() if not v]
    assert not missing, f"check kinds without breaking coverage: {missing}"


def test_no_applicable_mutants_is_not_a_pass():
    report = cc.criticize_task("empty", "OPENQASM 2.0;\nqreg q[1];\nh q[0];",
                               "bell_entangled", {})
    assert report["ok"] is False
