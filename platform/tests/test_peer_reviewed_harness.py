"""Unit tests for the peer-reviewed hardware harness scale validation."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))

import run_peer_reviewed_experiment as harness  # noqa: E402


def test_zne_scales_accepts_fractional_and_integer():
    assert harness._parse_zne_scales("1,1.5,2,3,4") == [1.0, 1.5, 2.0, 3.0, 4.0]


@pytest.mark.parametrize("raw", ["1,1.5,1.5,2", "1,2", "0.5,1,2", "1,2,3,4,5,6,7"])
def test_zne_scales_rejects_invalid_axis(raw):
    with pytest.raises(argparse.ArgumentTypeError):
        harness._parse_zne_scales(raw)
