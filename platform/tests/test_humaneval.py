"""HumanEval code-task corpus: licence gate, wrap contract, corpus integrity."""

import json
from pathlib import Path

import pytest

from kubona.eval.humaneval import load_code_tasks, wrap_for_harness

CORPUS = Path("kubona/eval/corpora/humaneval.json")
EXCLUDED = Path("kubona/eval/corpora/humaneval.excluded.json")


def test_corpus_loads_136_licence_clean():
    tasks = load_code_tasks(CORPUS)
    assert len(tasks) == 136
    meta = json.loads(CORPUS.read_text())["_meta"]
    assert "Apache-2.0" in meta["license"]
    assert meta["admitted"] == 136 and meta["excluded"] == 15


def test_every_entry_wraps_to_runnable_harness_pair():
    tasks = load_code_tasks(CORPUS)
    for name, entry in tasks.items():
        pair = wrap_for_harness(entry)
        assert f"from solution import {entry['entry_point']}" in pair["tests"]
        assert f"check({entry['entry_point']})" in pair["tests"]
        assert entry["reference"].strip()  # a reference body exists to golden-check
        assert entry["difficulty"] in ("basic", "intermediate", "difficult")


def test_exclusions_named_nothing_silent():
    excluded = json.loads(EXCLUDED.read_text())
    assert len(excluded) == 15
    assert all(isinstance(reason, str) and len(reason) > 20
               for reason in excluded.values())
    tasks = load_code_tasks(CORPUS)
    assert not (set(excluded) & set(tasks))  # excluded stay excluded


def test_licence_gate_refuses_code_corpus(tmp_path):
    bad = tmp_path / "bad.json"
    bad.write_text(json.dumps({"_meta": {"license": "proprietary"},
                               "t": {"prompt": "p", "tests": "t",
                                     "reference": "r", "entry_point": "f"}}))
    with pytest.raises(ValueError, match="refused"):
        load_code_tasks(bad)


def test_humaneval_quarantine_refuses_undeclared(tmp_path):
    import json as _json
    from kubona.eval.humaneval import load_code_tasks
    bad = tmp_path / "nometa.json"
    bad.write_text(_json.dumps({"_meta": {"license": "Apache-2.0"},
                                "t": {"prompt": "p", "tests": "t",
                                      "reference": "r", "entry_point": "f"}}))
    import pytest as _pytest
    with _pytest.raises(ValueError, match="quarantine"):
        load_code_tasks(bad)
