"""Eval corpus loader (G13 mechanism): declarative JSON corpora plug into the
evaluate_codegen contract, and every loaded task is verified to satisfiable."""

import json
from pathlib import Path

from kubona.eval.corpus_loader import load_corpus_dir
from kubona.eval.corpora import TASKS, evaluate_codegen
from kubona import config


def test_extended_corpus_loads_and_passes(tmp_path):
    import kubona.eval.corpora as corpora
    corpus_dir = Path(corpora.__file__).parent / "corpora"
    added = load_corpus_dir(corpus_dir)
    assert "ghz5" in added and "qft4" in added and "ghz6" in added

    # every loaded task must be satisfiable: feeding its own QASM through the
    # generator must pass its invariant on the simulator (proves not a fake corpus)
    results = evaluate_codegen(lambda name: corpora.TASKS[name]["qasm"],
                               provider="local-simulator", shots=16384)
    assert results["score"] == 1.0, results
    assert all(r["ok"] for r in results["tasks"].values() if r["qasm"].strip()[0] not in "<")


def test_corpus_loader_rejects_invalid_entries(tmp_path):
    import kubona.eval.corpora as corpora
    # a corpus with a bad-QASM entry and an unknown check kind is skipped cleanly
    d = tmp_path / "bad.json"
    d.write_text(json.dumps({
        "_meta": {"license": "Apache-2.0 (in-repo authored)", "source_url": "test-fixture"},
        "good-ghz2": {"qasm": "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];", "check": "ghz", "check_params": {"n": 2}},
        "bad-qasm": {"qasm": "this is not qasm", "check": "ghz", "check_params": {"n": 2}},
        "bad-check": {"qasm": "OPENQASM 2.0;\nqreg q[1];", "check": "nonexistent"},
    }))
    added = load_corpus_dir(tmp_path)
    assert "good-ghz2" in added
    assert "bad-qasm" not in added and "bad-check" not in added


def test_corpus_licence_gate(tmp_path):
    """Q-26: proprietary, unknown, or missing licences refuse the whole file."""
    import json
    from kubona.eval.corpus_loader import load_corpus_dir_with_report

    good = {"_meta": {"license": "CC0", "source_url": "test"},
            "t1": {"qasm": "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];",
                   "check": "ghz", "check_params": {"n": 2}}}
    (tmp_path / "good.json").write_text(json.dumps(good))
    (tmp_path / "prop.json").write_text(json.dumps({
        "_meta": {"license": "proprietary - do not redistribute"},
        "t2": dict(good["t1"])}))
    (tmp_path / "nometa.json").write_text(json.dumps({"t3": dict(good["t1"])}))
    (tmp_path / "mit.json").write_text(json.dumps(
        {"_meta": {"license": "MIT"}, "t4": dict(good["t1"])}))

    added, skipped = load_corpus_dir_with_report(tmp_path)
    assert "t1" in added and "t4" in added
    assert "t2" not in added and "t3" not in added
    assert "prop.json" in skipped and "nometa.json" in skipped
    assert "proprietary" in skipped["prop.json"]
