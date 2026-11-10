"""Bounded DMRG ground-state energies for open one-dimensional spin chains.

The solver is an honest matrix-product-state calculation: the Hamiltonian is
built as a nearest-neighbour MPO, and the returned energy is the variational
DMRG energy at the declared bond dimension and truncation cutoff. It is not an
exact proof, and no finite bond dimension carries a certified error bound.
"""

from __future__ import annotations

import math
from typing import Any

import numpy as np

SUPPORTED_DMRG_MODELS = frozenset({"tfim", "heisenberg", "xxz"})


def dmrg_ground_energy(
    model: str = "tfim",
    n_sites: int = 16,
    coupling: float = 1.0,
    field: float = 1.0,
    anisotropy: float = 1.0,
    bond_dim: int = 32,
    cutoff: float = 1e-10,
    sweeps: int = 8,
    tolerance: float = 1e-8,
) -> dict[str, Any]:
    """Return a real DMRG energy for an open spin chain.

    Quimb's spin operators are spin-1/2 operators, so the Pauli Hamiltonian
    used by ``SpinChainModel`` maps to factor 4 on two-site terms and factor 2
    on the transverse field. Open boundaries are deliberate: the MPO is
    nearest-neighbour and no periodic long-range term is hidden.
    """
    key = str(model or "").strip().lower()
    if key not in SUPPORTED_DMRG_MODELS:
        return {"success": False, "error": f"model must be one of {sorted(SUPPORTED_DMRG_MODELS)}, got {model!r}"}
    n = int(n_sites)
    if n < 4 or n > 40:
        return {"success": False, "error": f"n_sites must be in [4,40] for DMRG, got {n}"}
    chi = int(bond_dim)
    if chi < 4 or chi > 64:
        return {"success": False, "error": f"bond_dim must be in [4,64], got {chi}"}
    if int(sweeps) < 1 or int(sweeps) > 30:
        return {"success": False, "error": f"sweeps must be in [1,30], got {sweeps}"}
    if not math.isfinite(float(cutoff)) or float(cutoff) <= 0 or float(cutoff) > 1e-2:
        return {"success": False, "error": f"cutoff must be finite and in (0,1e-2], got {cutoff!r}"}
    if not math.isfinite(float(tolerance)) or float(tolerance) <= 0:
        return {"success": False, "error": f"tolerance must be finite and positive, got {tolerance!r}"}
    j = float(coupling)
    h = float(field)
    delta = float(anisotropy)
    if not all(math.isfinite(v) for v in (j, h, delta)):
        return {"success": False, "error": "coupling, field and anisotropy must be finite"}
    if key == "tfim":
        if j == 0 and h == 0:
            return {"success": False, "error": "tfim requires coupling or field to be non-zero"}
    elif key == "heisenberg":
        if j == 0:
            return {"success": False, "error": "heisenberg requires a non-zero coupling"}
    else:
        if j == 0:
            return {"success": False, "error": "xxz requires a non-zero coupling"}

    try:
        from quimb.tensor import DMRG2, SpinHam1D
        from quimb.tensor.tensor_core import TNLinearOperator

        if not getattr(TNLinearOperator, "_kubona_no_const_fold", False):
            _orig_tn_linop_init = TNLinearOperator.__init__

            def _kubona_tn_linop_init(self: Any, *args: Any, **kwargs: Any) -> None:
                _orig_tn_linop_init(self, *args, **kwargs)
                self._kws = {"get": "expression"}
                self._ins = tuple(t.data for t in self._tensors)

            TNLinearOperator.__init__ = _kubona_tn_linop_init  # type: ignore[method-assign]
            TNLinearOperator._kubona_no_const_fold = True  # type: ignore[attr-defined]
    except Exception as e:  # noqa: BLE001
        return {"success": False, "error": f"quimb DMRG is unavailable: {e}"}

    ham = SpinHam1D(S=0.5)
    if key == "tfim":
        ham.add_term(-4.0 * j, "Z", "Z")
        ham.add_term(-2.0 * h, "X")
    else:
        ham.add_term(4.0 * j, "X", "X")
        ham.add_term(4.0 * j, "Y", "Y")
        ham.add_term(4.0 * j * delta, "Z", "Z")
    try:
        mpo = ham.build_mpo(n)
        dmrg = DMRG2(mpo, bond_dims=chi, cutoffs=float(cutoff))
        converged = bool(dmrg.solve(
            tol=float(tolerance), max_sweeps=int(sweeps), verbosity=0,
            suppress_warnings=False,
        ))
        raw_energy = dmrg.energy
        energy = float(np.real(raw_energy))
        state = dmrg.state
        max_bond = int(state.max_bond())
    except Exception as e:  # noqa: BLE001
        return {"success": False, "error": f"DMRG failed: {e}"}
    if not math.isfinite(energy):
        return {"success": False, "error": "DMRG returned a non-finite energy"}
    return {
        "success": True,
        "model": key,
        "n_sites": n,
        "boundary": "open",
        "coupling": j,
        "field": h,
        "anisotropy": delta,
        "energy": energy,
        "energy_per_site": energy / n,
        "bond_dim": chi,
        "max_bond_reached": max_bond,
        "cutoff": float(cutoff),
        "sweeps_requested": int(sweeps),
        "converged": converged,
        "method": "DMRG2 (two-site, matrix product state)",
        "measured": False,
        "hardware": False,
        "claimable": False,
        "limitations": (
            "Variational MPS/DMRG energy at the declared bond dimension; "
            "open boundary; no certified truncation-error bound. "
            "This is a classical numerical baseline, not a measurement."
        ),
    }
