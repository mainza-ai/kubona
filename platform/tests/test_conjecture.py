import pytest

from kubona.science.conjecture import discover_sequence_laws


def test_fibonacci_recurrence_is_discovered_and_holdout_verified():
    out = discover_sequence_laws([1, 1, 2, 3, 5, 8, 13, 21, 34])
    assert out["success"] is True
    assert out["kind"] == "conjecture"
    assert out["result_version"] == "experiment-result-v1"
    assert out["candidate_count"] >= 1
    rec = next(c for c in out["candidates"] if c["kind"] == "linear_recurrence")
    assert rec["holdout_verified"] is True
    assert rec["status"] == "conjecture"
    assert rec["predictions"][:2] == ["55", "89"]


def test_square_sequence_polynomial_is_discovered():
    out = discover_sequence_laws([0, 1, 4, 9, 16, 25, 36, 49, 64], max_degree=3)
    poly = next(c for c in out["candidates"] if c["kind"] == "polynomial")
    assert poly["holdout_verified"] is True
    assert poly["predictions"][0] == "81"
    assert out["candidate_count"] == len({
        (c["kind"], c["law"]) for c in out["candidates"]
    })


def test_unpredictable_sequence_rejects_overfit():
    out = discover_sequence_laws([2, 7, 1, 8, 2, 8, 1, 8, 2], max_degree=6, max_order=4)
    assert out["success"] is True
    assert out["candidate_count"] == 0
    assert out["status"] == "no_law_verified"
    assert out["claimable"] is False


def test_exact_rational_inputs_and_holdout_required():
    out = discover_sequence_laws(["1/2", "3/2", "5/2", "7/2", "9/2", "11/2"], holdout=2)
    assert out["success"] is True
    assert out["candidate_count"] >= 1
    assert all(c["proof_status"] == "not-a-proof" for c in out["candidates"])


def test_bad_input_refuses():
    with pytest.raises(ValueError, match="3 sequence"):
        discover_sequence_laws([1, 2])
    with pytest.raises(ValueError, match="holdout"):
        discover_sequence_laws([1, 2, 3, 4, 5], holdout=4)
    with pytest.raises(ValueError, match="finite"):
        discover_sequence_laws([1, 2, float("nan")])
