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
        if matches!(lname, "include" | "creg" | "barrier" | "opaque" | "if") {
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
                .map(|x| x.trim().parse::<f64>().map_err(|_| "bad param"))
                .collect::<Result<Vec<_>, _>>()?,
            _ => Vec::new(),
        };
        let mut qubits = Vec::new();
        for q in q_part.split(',') {
            let q = q.trim();
            if let Some(o) = q.find('[') {
                let end = q.find(']').ok_or("bad qubit ref")?;
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
        let want_params = match name.as_str() {
            "u1" => 1,
            "u2" => 2,
            "u3" => 3,
            _ => 0,
        };
        if params.len() < want_params {
            return Err(format!("gate '{name}' needs {want_params} params, got {}", params.len()));
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

/// Decompose u1/u2/u3 into the native set (rz, ry, cx, ...).
pub fn transpile_to_native(c: &Circuit) -> Circuit {
    use std::f64::consts::PI;
    let mut gates = Vec::new();
    for g in &c.gates {
        let q = g.qubits[0];
        match g.name.as_str() {
            "u3" => {
                let (th, ph, la) = (g.params[0], g.params[1], g.params[2]);
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![la], cbit: None });
                gates.push(Gate { name: "ry".into(), qubits: vec![q], params: vec![th], cbit: None });
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![ph], cbit: None });
            }
            "u2" => {
                let (ph, la) = (g.params[0], g.params[1]);
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![la], cbit: None });
                gates.push(Gate { name: "ry".into(), qubits: vec![q], params: vec![PI / 2.0], cbit: None });
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![ph], cbit: None });
            }
            "u1" => {
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![g.params[0]], cbit: None });
            }
            _ => gates.push(g.clone()),
        }
    }
    Circuit { name: c.name.clone(), num_qubits: c.num_qubits, gates }
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
    fn decomposes_u3_to_native() {
        let c = parse("qreg q[1];\nu3(0.5, 0.2, 0.1) q[0];").unwrap();
        let n = transpile_to_native(&c);
        assert_eq!(n.gates.len(), 3);
        assert_eq!(n.gates[0].name, "rz");
        assert_eq!(n.gates[1].name, "ry");
        assert_eq!(n.gates[2].name, "rz");
    }

    #[test]
    fn emits_qir_calls() {
        let c = transpile_to_native(&parse(BELL).unwrap());
        let text = emit_llvm_ir(&c).unwrap();
        assert!(text.contains("@__quantum__qis__h__body"));
        assert!(text.contains("@__quantum__qis__cnot__body"));
        assert!(text.contains("@__quantum__rt__qubit_allocate"));
        assert!(text.contains("@__quantum__rt__record_output"));
    }

    #[test]
    fn decomposes_u1_u2_to_native() {
        let u1 = transpile_to_native(&parse("qreg q[1];\nu1(0.7) q[0];").unwrap());
        assert_eq!(u1.gates.len(), 1);
        assert_eq!(u1.gates[0].name, "rz");
        assert_eq!(u1.gates[0].params, vec![0.7]);
        let u2 = transpile_to_native(&parse("qreg q[1];\nu2(0.2, 0.9) q[0];").unwrap());
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
