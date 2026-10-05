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
        if let Some(rest) = line.strip_prefix("qreg") {
            let end = rest.find(']').ok_or("malformed qreg")?;
            let n: usize = rest[rest.find('[').ok_or("qreg missing [")? + 1..end]
                .trim()
                .parse()
                .map_err(|_| "bad qreg size")?;
            num_qubits = n;
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
        gates.push(Gate { name, qubits, params });
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
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![la] });
                gates.push(Gate { name: "ry".into(), qubits: vec![q], params: vec![th] });
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![ph] });
            }
            "u2" => {
                let (ph, la) = (g.params[0], g.params[1]);
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![la] });
                gates.push(Gate { name: "ry".into(), qubits: vec![q], params: vec![PI / 2.0] });
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![ph] });
            }
            "u1" => {
                gates.push(Gate { name: "rz".into(), qubits: vec![q], params: vec![g.params[0]] });
            }
            _ => gates.push(g.clone()),
        }
    }
    Circuit { name: c.name.clone(), num_qubits: c.num_qubits, gates }
}

/// Emit QIR-like textual LLVM IR.
pub fn emit_qir(c: &Circuit) -> String {
    let mut out = String::new();
    out.push_str("; QIR-like textual IR (qir-core, Rust reference compiler)\n");
    out.push_str(&format!("; circuit: {}  qubits: {}\n", c.name, c.num_qubits));
    out.push_str("define void @main() {\nentry:\n");
    for q in 0..c.num_qubits {
        out.push_str(&format!("  %q{q} = call %Qubit* @__quantum__rt__qubit_allocate()\n"));
    }
    for g in &c.gates {
        let qs: Vec<String> = g.qubits.iter().map(|q| format!("%q{q}")).collect();
        let qs = qs.join(", ");
        if g.params.is_empty() {
            out.push_str(&format!("  call void @__quantum__qis__{}__body({qs})\n", g.name));
        } else {
            let ps: Vec<String> = g.params.iter().map(|p| format!("{p:?}")).collect();
            out.push_str(&format!(
                "  call void @__quantum__qis__{}__body({}, {})\n",
                g.name,
                ps.join(", "),
                qs
            ));
        }
    }
    for q in 0..c.num_qubits {
        out.push_str(&format!("  call void @__quantum__rt__record_output({q})\n"));
    }
    out.push_str("  ret void\n}\n");
    out
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
        let text = emit_qir(&c);
        assert!(text.contains("@__quantum__qis__h__body"));
        assert!(text.contains("@__quantum__qis__cx__body"));
        assert!(text.contains("@__quantum__rt__qubit_allocate"));
        assert!(text.contains("@__quantum__rt__record_output"));
    }

    #[test]
    fn test_qft_4qubit_lowering_and_analysis() {
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

        let llvm_ir = emit_llvm_ir(&c);
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
