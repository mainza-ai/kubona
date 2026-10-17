"""P1-4 step 2, part 2 (freeze, no behavior change): the Python side of
the future single verdict schema.

Pins the DisconfirmationVerdict JSON wire shape (field names + types +
both polarities) so any future unification — Python models mirroring Go
verdict strings — has a baseline that fails loudly on drift. Makes NO
canonical choice about which Go vocabulary the finding-level verdict
maps to (operator decision, see the design page).
"""

from kubona.agents.collective import DisconfirmationOfficer


def test_falsified_wire_shape():
    v = DisconfirmationOfficer().challenge_finding(
        experiment_energy=-1.0, classical_anchor_energy=-1.1, qubits=2)
    d = v.model_dump()
    assert set(d) == {"is_falsified", "skeptic_notes",
                      "classical_shortcut_found", "evidence_defects"}
    assert d["is_falsified"] is True
    assert isinstance(d["skeptic_notes"], str) and d["skeptic_notes"]
    assert d["classical_shortcut_found"] is True
    assert isinstance(d["evidence_defects"], list) and len(d["evidence_defects"]) == 2
    # JSON round-trip: the shape a Go reader would consume.
    v2 = type(v).model_validate_json(v.model_dump_json())
    assert v2 == v


def test_withstood_wire_shape():
    v = DisconfirmationOfficer().challenge_finding(
        experiment_energy=-1.137, classical_anchor_energy=-1.1375, qubits=8)
    d = v.model_dump()
    assert d["is_falsified"] is False
    assert d["classical_shortcut_found"] is False
    assert d["evidence_defects"] == []
    assert type(v).model_validate_json(v.model_dump_json()) == v
