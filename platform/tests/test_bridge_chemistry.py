"""Bridge chemistry anchors: no silent molecule substitution, live families.

Regression cover for the H2-substitution bug (unknown molecules computed
H2 geometry, returned H2 energies labeled as the request, success:true)
and the H2O CASCI(6,6) crash. RDKit-dependent tests skip when rdkit is
absent (CI installs pyscf only); refusal + cascade tests always run.
"""

import importlib.util
import sys
from pathlib import Path

import pytest

BRIDGE = Path(__file__).resolve().parent.parent / "bridge"


def _load(name, filename):
    if str(BRIDGE) not in sys.path:
        sys.path.insert(0, str(BRIDGE))
    spec = importlib.util.spec_from_file_location(name, BRIDGE / filename)
    mod = importlib.util.module_from_spec(spec)
    sys.modules[name] = mod
    spec.loader.exec_module(mod)
    return mod


@pytest.fixture()
def chem():
    return _load("bridge_chemistry_under_test", "chemistry.py")


def test_unknown_molecule_refuses_never_substitutes(chem):
    # No RDKit needed: garbage names raise before any solver runs.
    with pytest.raises(RuntimeError, match="refusing instead of substituting"):
        chem.solve_classical_anchor("XenonX", 1.0)


def test_h2o_casci_cascade_succeeds(chem):
    pyscf = pytest.importorskip("pyscf")
    del pyscf
    out = chem.solve_classical_anchor("H2O", 0.9575)
    assert abs(out["rhf_energy"] - -74.96298) < 0.01, out
    assert abs(out["fci_exact_energy"] - -74.97040) < 0.02, out


def test_named_large_molecule_resolves_via_smiles(chem):
    pytest.importorskip("rdkit")
    pytest.importorskip("pyscf")
    out = chem.solve_classical_anchor("Benzene", 1.397)
    # Benzene-scale energy proves no H2 substitution (H2 FCI is -1.137).
    assert out["fci_exact_energy"] < -100, out
    assert out["molecule"] == "BENZENE"
