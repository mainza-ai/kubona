//! QIR LLVM IR Code Generation and Circuit Analysis.
//!
//! Complies with the QIR (Quantum Intermediate Representation) specification
//! (Base Profile / custom extensions) based on LLVM 14+ IR semantics.

use crate::Circuit;

#[derive(Debug, Clone, PartialEq)]
pub struct QirAnalysisSummary {
    pub instruction_count: usize,
    pub qubit_allocations: usize,
    pub gate_depth: usize,
    pub single_qubit_gates: usize,
    pub two_qubit_gates: usize,
    /// W1.5: 3+-qubit gates used to vanish from the counts (match 1/2/else).
    pub multi_qubit_gates: usize,
    pub total_gates: usize,
}

impl QirAnalysisSummary {
    pub fn to_json(&self) -> String {
        format!(
            "{{\"instruction_count\":{},\"qubit_allocations\":{},\"gate_depth\":{},\"single_qubit_gates\":{},\"two_qubit_gates\":{},\"multi_qubit_gates\":{},\"total_gates\":{}}}",
            self.instruction_count,
            self.qubit_allocations,
            self.gate_depth,
            self.single_qubit_gates,
            self.two_qubit_gates,
            self.multi_qubit_gates,
            self.total_gates
        )
    }
}

/// Analyze a Circuit to extract depth, gate categories, and instruction metrics.
pub fn analyze_circuit(c: &Circuit) -> QirAnalysisSummary {
    let mut qubit_layers = vec![0usize; c.num_qubits.max(1)];
    let mut single_q = 0usize;
    let mut two_q = 0usize;
    let mut multi_q = 0usize;

    for g in &c.gates {
        match g.qubits.len() {
            1 => single_q += 1,
            2 => two_q += 1,
            _ => multi_q += 1,
        }

        let max_prev = g.qubits.iter().map(|&q| if q < qubit_layers.len() { qubit_layers[q] } else { 0 }).max().unwrap_or(0);
        let next_layer = max_prev + 1;
        for &q in &g.qubits {
            if q < qubit_layers.len() {
                qubit_layers[q] = next_layer;
            }
        }
    }

    let gate_depth = qubit_layers.into_iter().max().unwrap_or(0);
    // Instruction count = allocations (n) + gate calls (g) + record outputs (n) + releases (n) + ret (1)
    let instruction_count = c.num_qubits * 3 + c.gates.len() + 1;

    QirAnalysisSummary {
        instruction_count,
        qubit_allocations: c.num_qubits,
        gate_depth,
        single_qubit_gates: single_q,
        two_qubit_gates: two_q,
        multi_qubit_gates: multi_q,
        total_gates: c.gates.len(),
    }
}

/// Closed emission set (W1.5): every other gate name refuses. The old
/// emitter interpolated ANY name into @__quantum__qis__{name}__body with
/// only fixed declares — arbitrary QASM emitted un-linkable IR.
fn qis_intrinsic(name: &str) -> Option<&'static str> {
    Some(match name {
        "h" => "h",
        "x" => "x",
        "y" => "y",
        "z" => "z",
        "s" => "s",
        "t" => "t",
        "rx" => "rx",
        "ry" => "ry",
        "rz" => "rz",
        "cx" => "cnot",
        "cz" => "cz",
        "swap" => "swap",
        "reset" => "reset",
        _ => return None,
    })
}

/// Generate compliant QIR LLVM text IR for a quantum circuit.
/// Refuses unknown gates and malformed measurements: invalid IR is never
/// emitted (callers surface the error; main exits 1).
pub fn emit_llvm_ir(c: &Circuit) -> Result<String, String> {
    let mut out = String::with_capacity(4096);

    // LLVM Module Headers
    out.push_str("; ModuleID = 'kubona_qir_module'\n");
    out.push_str(&format!("source_filename = \"{}.qir\"\n", c.name));
    out.push_str("target datalayout = \"e-m:e-p270:32:32-p271:32:32-p272:64:64-i64:64-f80:128-n8:16:32:64-S128\"\n");
    out.push_str("target triple = \"x86_64-unknown-linux-gnu\"\n\n");

    // Opaque Types
    out.push_str("%Qubit = type opaque\n");
    out.push_str("%Result = type opaque\n\n");

    // Quantum Runtime Declarations
    out.push_str("; Quantum Runtime Functions\n");
    out.push_str("declare %Qubit* @__quantum__rt__qubit_allocate()\n");
    out.push_str("declare void @__quantum__rt__qubit_release(%Qubit*)\n");
    out.push_str("declare void @__quantum__rt__record_output(i64, %Result*)\n\n");

    // Quantum Instruction Set (QIS) Declarations
    out.push_str("; Quantum Instruction Set Functions\n");
    out.push_str("declare void @__quantum__qis__h__body(%Qubit*)\n");
    out.push_str("declare void @__quantum__qis__x__body(%Qubit*)\n");
    out.push_str("declare void @__quantum__qis__y__body(%Qubit*)\n");
    out.push_str("declare void @__quantum__qis__z__body(%Qubit*)\n");
    out.push_str("declare void @__quantum__qis__s__body(%Qubit*)\n");
    out.push_str("declare void @__quantum__qis__t__body(%Qubit*)\n");
    out.push_str("declare void @__quantum__qis__cnot__body(%Qubit*, %Qubit*)\n");
    out.push_str("declare void @__quantum__qis__cz__body(%Qubit*, %Qubit*)\n");
    out.push_str("declare void @__quantum__qis__swap__body(%Qubit*, %Qubit*)\n");
    out.push_str("declare void @__quantum__qis__rx__body(double, %Qubit*)\n");
    out.push_str("declare void @__quantum__qis__ry__body(double, %Qubit*)\n");
    out.push_str("declare void @__quantum__qis__rz__body(double, %Qubit*)\n");
    out.push_str("declare void @__quantum__qis__reset__body(%Qubit*)\n");
    out.push_str("declare %Result* @__quantum__qis__mz__body(%Qubit*)\n\n");

    // Main Entrypoint
    out.push_str("define void @main() #0 {\n");
    out.push_str("entry:\n");

    // Qubit allocations
    for q in 0..c.num_qubits {
        out.push_str(&format!("  %q{q} = call %Qubit* @__quantum__rt__qubit_allocate()\n"));
    }

    // Gate invocations through the closed intrinsic set. `id` emits
    // nothing (identity, validated at parse). Measurements thread their
    // classical bit: %res_{idx} names the gate result (W1.5 — the old code
    // dropped the bit and called an undeclared measure intrinsic).
    for (idx, g) in c.gates.iter().enumerate() {
        let gate_name = g.name.as_str();
        if gate_name == "id" {
            continue;
        }
        if gate_name == "measure" || gate_name == "mz" || gate_name == "m" {
            let q = g.qubits.first().copied().ok_or_else(|| format!("measure gate {idx} addresses no qubit"))?;
            out.push_str(&format!(
                "  %res_{idx} = call %Result* @__quantum__qis__mz__body(%Qubit* %q{q})\n"
            ));
            continue;
        }
        let intrinsic = qis_intrinsic(gate_name)
            .ok_or_else(|| format!("gate '{gate_name}' is outside the QIR emission set: refusing"))?;
        if g.params.is_empty() {
            let qs: Vec<String> = g.qubits.iter().map(|q| format!("%Qubit* %q{q}")).collect();
            out.push_str(&format!("  call void @__quantum__qis__{intrinsic}__body({})\n", qs.join(", ")));
        } else {
            let ps: Vec<String> = g.params.iter().map(|p| format!("double {p:?}")).collect();
            let qs: Vec<String> = g.qubits.iter().map(|q| format!("%Qubit* %q{q}")).collect();
            out.push_str(&format!(
                "  call void @__quantum__qis__{intrinsic}__body({}, {})\n",
                ps.join(", "),
                qs.join(", ")
            ));
        }
    }

    // Output recording
    for q in 0..c.num_qubits {
        out.push_str(&format!(
            "  %r{q} = call %Result* @__quantum__qis__mz__body(%Qubit* %q{q})\n"
        ));
        out.push_str(&format!(
            "  call void @__quantum__rt__record_output(i64 {q}, %Result* %r{q})\n"
        ));
    }

    // Qubit releases
    for q in 0..c.num_qubits {
        out.push_str(&format!("  call void @__quantum__rt__qubit_release(%Qubit* %q{q})\n"));
    }

    out.push_str("  ret void\n");
    out.push_str("}\n\n");

    // Attributes and Metadata
    out.push_str("attributes #0 = { \"entry_point\" \"qir_profiles\"=\"BaseProfile\" \"output_labeling_schema\"=\"v1\" }\n\n");
    out.push_str("!llvm.module.flags = !{!0, !1}\n");
    out.push_str("!0 = !{i32 1, !\"qir_major_version\", i32 1}\n");
    out.push_str("!1 = !{i32 7, !\"qir_minor_version\", i32 0}\n");

    Ok(out)
}
