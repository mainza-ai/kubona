//! qir-core — the Rust lowering core for Kubona (Phase 1 reference compiler).
//!
//! Scope: parse a QASM 2/3 subset, decompose high-level gates to a native set,
//! and emit QIR-like textual LLVM IR. The production target is QIR bitcode
//! (Phase 1 hardening) and MLIR dialects (later phases). No external deps.

use std::fmt;

#[derive(Debug, Clone, PartialEq)]
pub struct Gate {
    pub name: String,
    pub qubits: Vec<usize>,
    pub params: Vec<f64>,
    /// Classical target bit for `measure q[i] -> c[j]` (None otherwise).
    /// W1.5: the old parser dropped the bit and the emitter called an
    /// undeclared @__quantum__qis__measure__body — invalid IR.
    pub cbit: Option<usize>,
}

#[derive(Debug, Clone, PartialEq)]
pub struct Circuit {
    pub name: String,
    pub num_qubits: usize,
    pub gates: Vec<Gate>,
}

/// Parse a QASM 2/3 subset: `qreg q[n];` plus gate lines
/// `name[(p, ...)] q[a], q[b];` (lines and `//` comments).
pub fn parse(source: &str) -> Result<Circuit, String> {
    let mut num_qubits = 0usize;
    let mut gates = Vec::new();
    for raw in source.lines() {
        let line = match raw.find("//") {
            Some(i) => &raw[..i],
            None => raw,
        }
        .trim();
        if line.is_empty() || line.starts_with("OPENQASM") {
            continue;
        }
        // W1.5: structural directives are not gates. The old parser lowered
        // `creg`/`barrier` into calls to undeclared @__quantum__qis__creg__body
        // et al. — invalid IR from valid QASM.
        let lname = line.split_whitespace().next().unwrap_or("");
        if matches!(lname, "include" | "creg" | "barrier" | "opaque" | "if") || lname.starts_with("bit") {
            if lname == "opaque" || lname == "if" {
                return Err(format!("unsupported statement '{lname}': refusing"));
            }
            continue;
        }
        if let Some(rest) = line.strip_prefix("qreg") {
            let end = rest.find(']').ok_or("malformed qreg")?;
            let n: usize = rest[rest.find('[').ok_or("qreg missing [")? + 1..end]
                .trim()
                .parse()
                .map_err(|_| "bad qreg size")?;
            // W1.5: max width across registers (the old overwrite took the
            // last qreg, shrinking multi-register programs).
            num_qubits = num_qubits.max(n);
            continue;
        }
        if let Some(rest) = line.strip_prefix("qubit") {
            let rest = rest.trim();
            let n: usize = if rest.starts_with('[') {
                let end = rest.find(']').ok_or("malformed qubit declaration")?;
                rest[1..end].trim().parse().map_err(|_| "bad qubit register size")?
            } else {
                1
            };
            num_qubits = num_qubits.max(n);
            continue;
        }
        // gate line: name[(params)] q[a], q[b];
        let body = line.strip_suffix(';').ok_or_else(|| format!("missing ';' in {line}"))?;
        let open = body.find('(');
        let (name, param_part, q_part) = if let Some(o) = open {
            let close = body.find(')').ok_or("missing ')'")?;
            (&body[..o], Some(&body[o + 1..close]), &body[close + 1..])
        } else {
            let sp = body.find(' ').ok_or_else(|| format!("bad gate line {line}"))?;
            (&body[..sp], None, &body[sp..])
        };
        let name = name.trim().to_string();
        let params = match param_part {
            Some(p) if !p.trim().is_empty() => p
                .split(',')
                .map(|x| {
                    let v = x.trim().parse::<f64>().map_err(|_| "bad param")?;
                    if v.is_nan() || v.is_infinite() {
                        return Err("bad param: NaN or infinite value".into());
                    }
                    Ok(v)
                })
                .collect::<Result<Vec<_>, String>>()?,
            _ => Vec::new(),
        };
        let mut qubits = Vec::new();
        for q in q_part.split(',') {
            let q = q.trim();
            if let Some(o) = q.find('[') {
                let end = q.find(']').ok_or("bad qubit ref")?;
                if o >= end {
                    return Err("bad qubit ref".into());
                }
                qubits.push(q[o + 1..end].trim().parse::<usize>().map_err(|_| "bad qubit")?);
            }
        }
        // W1.5: measure threads its classical bit (q[i] -> c[j]) instead of
        // dropping it. Malformed measure refuses here, not in codegen.
        let mut cbit = None;
        if name == "measure" || name == "mz" || name == "m" {
            let arrow = q_part.find("->").ok_or("measure needs q[i] -> c[j]")?;
            let cpart = q_part[arrow + 2..].trim();
            let (co, ce) = (cpart.find('[').ok_or("bad classical bit")?, cpart.find(']').ok_or("bad classical bit")?);
            cbit = Some(cpart[co + 1..ce].trim().parse::<usize>().map_err(|_| "bad classical bit")?);
            if qubits.len() != 1 {
                return Err("measure takes exactly 1 qubit".into());
            }
        }
        // Fail-closed boundary: transpile indexes qubits[0] and params[0..2],
        // so a gate that parses but addresses nothing (or under-supplies a
        // u-gate) would panic downstream. Refuse here instead.
        if qubits.is_empty() {
            return Err(format!("gate '{name}' addresses no qubits"));
        }
        if qubits.len() > 1 {
            let mut seen = std::collections::HashSet::new();
            for q in &qubits {
                if !seen.insert(*q) {
                    return Err(format!("gate '{name}' has duplicate qubit operands: q[{q}]"));
                }
            }
        }
        let want_qubits = match name.as_str() {
            "h" | "x" | "y" | "z" | "s" | "t" | "rx" | "ry" | "rz" | "u1" | "u2" | "u3" | "id" | "reset" | "measure" | "mz" | "m" => Some(1),
            "cx" | "cz" | "swap" | "cnot" => Some(2),
            "ccx" | "toffoli" => Some(3),
            _ => None,
        };
        if let Some(expected) = want_qubits {
            if qubits.len() != expected {
                return Err(format!("gate '{name}' expects {expected} qubits, got {}", qubits.len()));
            }
        }
        let want_params = match name.as_str() {
            "u1" | "rx" | "ry" | "rz" => Some(1),
            "u2" => Some(2),
            "u3" => Some(3),
            "h" | "x" | "y" | "z" | "s" | "t" | "cx" | "cz" | "swap" | "cnot" | "id" | "reset" => Some(0),
            _ => None,
        };
        if let Some(expected) = want_params {
            if params.len() != expected {
                return Err(format!("gate '{name}' needs {expected} params, got {}", params.len()));
            }
        }
        gates.push(Gate { name, qubits, params, cbit });
    }
    // W1.5: max register width (the old overwrite took the LAST qreg), and
    // qubit bounds — %q5 with qreg q[2] used to emit undeclared SSA names.
    let max_idx = gates.iter().flat_map(|g| g.qubits.iter().copied()).max();
    if let Some(m) = max_idx {
        if m >= num_qubits {
            return Err(format!("qubit q[{m}] out of range for {num_qubits} declared qubits"));
        }
    }
    Ok(Circuit { name: "qasm".into(), num_qubits, gates })
}

/// Decompose u1/u2/u3 (and ccx/toffoli) into the native set (rz, ry, cx, ...).
///
/// Fallible: `parse` guarantees gate shapes, but a hand-constructed
/// `Circuit` may omit qubits/params — refuse instead of panicking on
/// `qubits[0]` / `params[0..2]`. Gates this pass does not consume are
/// forwarded unchanged; unknown names still refuse in `emit_llvm_ir`
/// (fail-closed), so pipeline output is always coherent.
pub fn transpile_to_native(c: &Circuit) -> Result<Circuit, String> {
    use std::f64::consts::PI;
    let mut gates = Vec::new();
    for (idx, g) in c.gates.iter().enumerate() {
        let q = *g.qubits.first().ok_or_else(|| {
            format!("transpile: gate {idx} ('{}') addresses no qubits: refusing", g.name)
        })?;
        match g.name.as_str() {
            "u3" => {
                if g.params.len() != 3 {
                    return Err(format!(
                        "transpile: u3 needs 3 params, got {}: refusing",
                        g.params.len()
                    ));
                }
                let (th, ph, la) = (g.params[0], g.params[1], g.params[2]);
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![la], cbit: None });
                gates.push(Gate { name: "ry".into(), qubits: vec![q], params: vec![th], cbit: None });
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![ph], cbit: None });
            }
            "u2" => {
                if g.params.len() != 2 {
                    return Err(format!(
                        "transpile: u2 needs 2 params, got {}: refusing",
                        g.params.len()
                    ));
                }
                let (ph, la) = (g.params[0], g.params[1]);
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![la], cbit: None });
                gates.push(Gate { name: "ry".into(), qubits: vec![q], params: vec![PI / 2.0], cbit: None });
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![ph], cbit: None });
            }
            "u1" => {
                if g.params.len() != 1 {
                    return Err(format!(
                        "transpile: u1 needs 1 param, got {}: refusing",
                        g.params.len()
                    ));
                }
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![g.params[0]], cbit: None });
            }
            // Toffoli has no BaseProfile QIS intrinsic, so (like u1/u2/u3)
            // it is decomposed here: the parse -> transpile -> emit pipeline
            // stays coherent, while a direct `emit_llvm_ir` of ccx still
            // refuses (closed 1q/2q emission set). Decomposition is Nielsen
            // & Chuang Fig. 4.9 with T = rz(pi/4), Tdg = rz(-pi/4): each
            // differs from the textbook gate by a single-qubit global phase
            // only, which is physically irrelevant.
            "ccx" | "toffoli" => {
                if g.qubits.len() != 3 {
                    return Err(format!(
                        "transpile: '{}' needs 3 qubits, got {}: refusing",
                        g.name,
                        g.qubits.len()
                    ));
                }
                let (c1, c2, t) = (g.qubits[0], g.qubits[1], g.qubits[2]);
                let rz = |q: usize, a: f64| Gate { name: "rz".into(), qubits: vec![q], params: vec![a], cbit: None };
                let cx = |a: usize, b: usize| Gate { name: "cx".into(), qubits: vec![a, b], params: vec![], cbit: None };
                let h = |q: usize| Gate { name: "h".into(), qubits: vec![q], params: vec![], cbit: None };
                gates.push(h(t));
                gates.push(cx(c2, t));
                gates.push(rz(t, -PI / 4.0));
                gates.push(cx(c1, t));
                gates.push(rz(t, PI / 4.0));
                gates.push(cx(c2, t));
                gates.push(rz(t, -PI / 4.0));
                gates.push(cx(c1, t));
                gates.push(rz(c2, PI / 4.0));
                gates.push(rz(t, PI / 4.0));
                gates.push(h(t));
                gates.push(cx(c1, c2));
                gates.push(rz(c1, PI / 4.0));
                gates.push(rz(c2, -PI / 4.0));
                gates.push(cx(c1, c2));
            }
            _ => gates.push(g.clone()),
        }
    }
    Ok(Circuit { name: c.name.clone(), num_qubits: c.num_qubits, gates })
}

pub mod codegen;
pub use codegen::{analyze_circuit, emit_llvm_ir, QirAnalysisSummary};

impl fmt::Display for Circuit {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{} qubits, {} gates", self.num_qubits, self.gates.len())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const BELL: &str = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];";

    #[test]
    fn parses_bell() {
        let c = parse(BELL).unwrap();
        assert_eq!(c.num_qubits, 2);
        assert_eq!(c.gates.len(), 2);
        assert_eq!(c.gates[0].name, "h");
    }

    #[test]
    fn parses_openqasm3_subset() {
        let qasm = "OPENQASM 3.0;\nqubit[2] q;\nbit[2] c;\nh q[0];\ncx q[0], q[1];\nreset q[0];";
        let c = parse(qasm).unwrap();
        assert_eq!(c.num_qubits, 2);
        assert_eq!(c.gates.len(), 3);
        assert_eq!(c.gates[2].name, "reset");
        let ir = emit_llvm_ir(&c).unwrap();
        assert!(ir.contains("@__quantum__qis__reset__body"));
    }

    #[test]
    fn decomposes_u3_to_native() {
        let c = parse("qreg q[1];\nu3(0.5, 0.2, 0.1) q[0];").unwrap();
        let n = transpile_to_native(&c).unwrap();
        assert_eq!(n.gates.len(), 3);
        assert_eq!(n.gates[0].name, "rz");
        assert_eq!(n.gates[1].name, "ry");
        assert_eq!(n.gates[2].name, "rz");
    }

    #[test]
    fn emits_qir_calls() {
        let c = transpile_to_native(&parse(BELL).unwrap()).unwrap();
        let text = emit_llvm_ir(&c).unwrap();
        assert!(text.contains("@__quantum__qis__h__body"));
        assert!(text.contains("@__quantum__qis__cnot__body"));
        assert!(text.contains("@__quantum__rt__qubit_allocate"));
        assert!(text.contains("@__quantum__rt__record_output"));
    }

    #[test]
    fn decomposes_u1_u2_to_native() {
        let u1 = transpile_to_native(&parse("qreg q[1];\nu1(0.7) q[0];").unwrap()).unwrap();
        assert_eq!(u1.gates.len(), 1);
        assert_eq!(u1.gates[0].name, "rz");
        assert_eq!(u1.gates[0].params, vec![0.7]);
        let u2 = transpile_to_native(&parse("qreg q[1];\nu2(0.2, 0.9) q[0];").unwrap()).unwrap();
        assert_eq!(u2.gates.len(), 3);
        assert_eq!(u2.gates[1].name, "ry");
        assert!((u2.gates[1].params[0] - std::f64::consts::PI / 2.0).abs() < 1e-12);
    }

    #[test]
    fn parse_refuses_malformed() {
        for bad in [
            "qreg q[2;\nh q[0];",          // malformed qreg
            "qreg q;\nh q[0];",            // qreg missing [
            "qreg q[x];\nh q[0];",         // bad qreg size
            "qreg q[1];\nh q[0]",          // missing ';'
            "qreg q[1];\nrz(foo) q[0];",   // bad param
            "qreg q[1];\nh q[x];",         // bad qubit
            "qreg q[1];\nrz(0.1 q[0];",    // missing ')'
            "qreg q[1];\nh;",               // gate addresses no qubits
            "qreg q[1];\nu3(0.5) q[0];",   // u3 needs 3 params
            "qreg q[1];\nu2(0.5) q[0];",   // u2 needs 2 params
            "qreg q[2];\ncx q[0], q[0];",  // duplicate qubit operands
            "qreg q[1];\nrz(NaN) q[0];",   // NaN param
            "qreg q[2];\ncx q[0];",        // cx needs 2 qubits
            "qreg q[2];\nh q[0], q[1];",   // h needs 1 qubit
            "qreg q[1];\nh(0.5) q[0];",    // h takes 0 params
        ] {
            assert!(parse(bad).is_err(), "must refuse: {bad}");
        }
    }

    #[test]
    fn parse_ignores_comments_and_headers() {
        let c = parse("// lead\nOPENQASM 2.0;\nqreg q[1]; // trailing\nh q[0]; // gate\n").unwrap();
        assert_eq!(c.gates.len(), 1);
        assert_eq!(c.gates[0].name, "h");
    }

    #[test]
    fn empty_circuit_emits_valid_skeleton() {
        let text = emit_llvm_ir(&parse("qreg q[1];").unwrap()).unwrap();
        assert!(text.contains("define void @main()"));
        assert!(text.contains("ret void"));
        // Declares are always present (closed set); no gate CALLS may appear.
        assert!(!text.contains("call void @__quantum__qis__"));
    }

    #[test]
    fn test_qft_4qubit_lowering_and_analysis() {
        // 4-qubit Quantum Fourier Transform circuit subset
        // 4-qubit Quantum Fourier Transform circuit subset
        let qft_src = "OPENQASM 2.0;\nqreg q[4];\nh q[0];\nrz(1.570796) q[0];\ncx q[1],q[0];\nh q[1];\nrz(0.785398) q[0];\ncx q[2],q[0];\nh q[2];\nh q[3];";
        let c = parse(qft_src).unwrap();
        assert_eq!(c.num_qubits, 4);

        let summary = analyze_circuit(&c);
        assert_eq!(summary.qubit_allocations, 4);
        assert_eq!(summary.total_gates, 8);
        assert_eq!(summary.two_qubit_gates, 2);
        assert_eq!(summary.single_qubit_gates, 6);
        assert!(summary.gate_depth > 0);
        assert!(summary.instruction_count > 0);

        let llvm_ir = emit_llvm_ir(&c).unwrap();
        assert!(llvm_ir.contains("target datalayout"));
        assert!(llvm_ir.contains("%Qubit = type opaque"));
        assert!(llvm_ir.contains("@__quantum__qis__h__body"));
        assert!(llvm_ir.contains("@__quantum__qis__cnot__body"));
        assert!(llvm_ir.contains("@__quantum__qis__rz__body"));
        assert!(llvm_ir.contains("@__quantum__rt__qubit_release"));
        assert!(llvm_ir.contains("\"qir_profiles\"=\"BaseProfile\""));
        assert!(llvm_ir.contains("qir_major_version"));

        let json_str = summary.to_json();
        assert!(json_str.contains("\"qubit_allocations\":4"));
        assert!(json_str.contains("\"two_qubit_gates\":2"));
    }
}

#[cfg(test)]
mod w15_tests {
    use super::*;

    #[test]
    fn measure_threads_classical_bit() {
        let c = parse("qreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];").unwrap();
        assert_eq!(c.gates[1].cbit, Some(0));
        let text = emit_llvm_ir(&c).unwrap();
        assert!(text.contains("@__quantum__qis__mz__body"));
        assert!(!text.contains("@__quantum__qis__measure__body"));
    }

    #[test]
    fn closed_set_refuses() {
        for bad in [
            "qreg q[1];\nfoo q[0];",
            "qreg q[1];\nch q[0];",
            "qreg q[3];\nccx q[0],q[1],q[2];",
            "qreg q[1];\nh q[5];",
        ] {
            // Refusal may land at parse (bounds/shape) or at emission
            // (closed set) — either way no invalid IR ships.
            match parse(bad) {
                Err(_) => (),
                Ok(c) => assert!(emit_llvm_ir(&c).is_err(), "must refuse: {bad}"),
            }
        }
        // Malformed measure refuses at parse (no classical target).
        assert!(parse("qreg q[1];\ncreg c[1];\nmeasure q[0];").is_err());
    }

    #[test]
    fn directives_are_not_gates() {
        let c = parse("OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[1];\ncreg c[1];\nbarrier q[0];\nh q[0];").unwrap();
        assert_eq!(c.gates.len(), 1);
        let text = emit_llvm_ir(&c).unwrap();
        assert!(!text.contains("creg"));
        assert!(!text.contains("barrier"));
    }

    #[test]
    fn multi_register_width_is_max() {
        let c = parse("qreg a[2];\nqreg b[3];\nh a[0];").unwrap();
        assert_eq!(c.num_qubits, 3);
    }

    #[test]
    fn multi_qubit_gates_counted() {
        let c = parse("qreg q[3];\nccx q[0],q[1],q[2];").unwrap();
        let s = analyze_circuit(&c);
        assert_eq!(s.multi_qubit_gates, 1);
        assert_eq!(s.total_gates, 1);
    }
}

#[cfg(test)]
mod phase1_tests {
    use super::*;

    fn mz_calls(ir: &str) -> usize {
        ir.matches("call %Result* @__quantum__qis__mz__body").count()
    }

    #[test]
    fn measure_routes_classical_bit_end_to_end() {
        // Single non-identity mapping: q[0] -> c[1].
        let c = parse("qreg q[2];\ncreg c[2];\nmeasure q[0] -> c[1];").unwrap();
        assert_eq!(c.gates[0].cbit, Some(1));
        let ir = emit_llvm_ir(&c).unwrap();
        // Exactly one mz (the declare line is not a call), on q[0] ...
        assert_eq!(mz_calls(&ir), 1);
        assert!(ir.contains("%res_0 = call %Result* @__quantum__qis__mz__body(%Qubit* %q0)"));
        // ... recorded under the CLASSICAL index ...
        assert!(ir.contains("call void @__quantum__rt__record_output(i64 1, %Result* %res_0)"));
        // ... and no duplicate full sweep.
        assert!(!ir.contains("%r0 = call"));
        assert!(!ir.contains("%r1 = call"));

        // Multi-measure with non-identity mapping: q[0]->c[1], q[2]->c[0].
        let c2 = parse(
            "qreg q[3];\ncreg c[3];\nh q[0];\nmeasure q[0] -> c[1];\nmeasure q[2] -> c[0];",
        )
        .unwrap();
        let ir2 = emit_llvm_ir(&c2).unwrap();
        assert_eq!(mz_calls(&ir2), 2);
        assert!(ir2.contains("@__quantum__qis__mz__body(%Qubit* %q0)"));
        assert!(ir2.contains("@__quantum__qis__mz__body(%Qubit* %q2)"));
        assert!(!ir2.contains("@__quantum__qis__mz__body(%Qubit* %q1)"));
        assert!(ir2.contains("call void @__quantum__rt__record_output(i64 1, %Result* %res_1)"));
        assert!(ir2.contains("call void @__quantum__rt__record_output(i64 0, %Result* %res_2)"));
        for r in ["%r0 = call", "%r1 = call", "%r2 = call"] {
            assert!(!ir2.contains(r), "duplicate sweep line present: {r}");
        }
        // Instruction count mirrors emission: alloc(3) + release(3) + ret(1)
        // + h(1) + 2 measures (2 each), no sweep.
        assert_eq!(analyze_circuit(&c2).instruction_count, 12);
    }

    #[test]
    fn measure_aliases_route_classical_bit() {
        for alias in ["mz", "m"] {
            let src = format!("qreg q[2];\ncreg c[2];\n{alias} q[1] -> c[0];");
            let c = parse(&src).unwrap();
            assert_eq!(c.gates[0].cbit, Some(0));
            let ir = emit_llvm_ir(&c).unwrap();
            assert_eq!(mz_calls(&ir), 1);
            assert!(ir.contains("@__quantum__qis__mz__body(%Qubit* %q1)"));
            assert!(ir.contains("call void @__quantum__rt__record_output(i64 0, %Result* %res_0)"));
        }
    }

    #[test]
    fn unmeasured_circuits_keep_identity_sweep() {
        // No explicit measure: legacy full sweep (each qubit to its own bit).
        let c = parse("qreg q[2];\nh q[0];").unwrap();
        let ir = emit_llvm_ir(&c).unwrap();
        assert_eq!(mz_calls(&ir), 2);
        assert!(ir.contains("call void @__quantum__rt__record_output(i64 0, %Result* %r0)"));
        assert!(ir.contains("call void @__quantum__rt__record_output(i64 1, %Result* %r1)"));
    }

    #[test]
    fn cnot_alias_emits_directly() {
        let c = parse("qreg q[2];\ncnot q[0],q[1];").unwrap();
        assert_eq!(c.gates[0].qubits, vec![0, 1]);
        let native = transpile_to_native(&c).unwrap();
        let ir = emit_llvm_ir(&native).unwrap();
        assert!(ir.contains("call void @__quantum__qis__cnot__body(%Qubit* %q0, %Qubit* %q1)"));
    }

    #[test]
    fn ccx_toffoli_decompose_through_pipeline() {
        // 3q Toffoli has no BaseProfile intrinsic: transpile decomposes it
        // (N&C Fig. 4.9 over h/cx/rz) so parse -> transpile -> emit is
        // coherent, while a direct emit still refuses (closed 1q/2q set —
        // same fail-closed shape as u1/u2/u3).
        for gate in ["ccx", "toffoli"] {
            let src = format!("qreg q[3];\n{gate} q[0],q[1],q[2];");
            let parsed = parse(&src).unwrap();
            let native = transpile_to_native(&parsed).unwrap();
            assert!(native.gates.len() > 1, "{gate} must decompose");
            assert!(
                native.gates.iter().all(|g| g.qubits.len() <= 2),
                "{gate} must decompose to 1q/2q gates"
            );
            let ir = emit_llvm_ir(&native).unwrap();
            assert!(ir.contains("@__quantum__qis__cnot__body"));
            assert!(ir.contains("@__quantum__qis__rz__body"));
            assert!(ir.contains("@__quantum__qis__h__body"));
            assert!(!ir.contains("ccx"), "{gate} must not survive lowering");
            assert!(!ir.contains("toffoli"), "{gate} must not survive lowering");
        }
    }

    #[test]
    fn transpile_refuses_hand_built_malformed() {
        let mk = |name: &str, qubits: Vec<usize>, params: Vec<f64>| Circuit {
            name: "evil".into(),
            num_qubits: 3,
            gates: vec![Gate { name: name.into(), qubits, params, cbit: None }],
        };
        // Empty qubit list (used to index qubits[0] -> panic).
        assert!(transpile_to_native(&mk("h", vec![], vec![])).is_err());
        // Under-supplied params (used to index params[0..2] -> panic).
        assert!(transpile_to_native(&mk("u3", vec![0], vec![0.5])).is_err());
        assert!(transpile_to_native(&mk("u2", vec![0], vec![0.5])).is_err());
        assert!(transpile_to_native(&mk("u1", vec![0], vec![])).is_err());
        // Toffoli shape violation.
        assert!(transpile_to_native(&mk("ccx", vec![0, 1], vec![])).is_err());
        assert!(transpile_to_native(&mk("toffoli", vec![0], vec![])).is_err());
        // Well-formed hand-built circuits still transpile.
        let ok = transpile_to_native(&mk("u3", vec![0], vec![0.5, 0.2, 0.1])).unwrap();
        assert_eq!(ok.gates.len(), 3);
        let ok_ccx = transpile_to_native(&mk("ccx", vec![0, 1, 2], vec![])).unwrap();
        assert!(ok_ccx.gates.len() > 1);
        let ok_plain = transpile_to_native(&mk("h", vec![1], vec![])).unwrap();
        assert_eq!(ok_plain.gates.len(), 1);
    }
}
