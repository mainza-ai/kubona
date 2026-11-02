//! qir-core CLI: read QASM from a file or stdin, emit QIR LLVM IR or JSON metrics.
//!
//! Usage: qir-core [--json] [--llvm] [file.qasm]   (reads stdin when no file given)

use std::collections::BTreeMap;
use std::io::{self, Read};

fn main() {
    let mut json_mode = false;
    let mut file_path = None;

    // W1.5: unknown flags refuse (usage error) instead of silently ignored.
    for arg in std::env::args().skip(1) {
        match arg.as_str() {
            "--json" => json_mode = true,
            "--llvm" => json_mode = false,
            other if !other.starts_with("--") => {
                if file_path.is_some() {
                    eprintln!("unexpected extra argument: {other}");
                    eprintln!("usage: qir-core [--json] [--llvm] [file.qasm]");
                    std::process::exit(2);
                }
                file_path = Some(other.to_string());
            }
            other => {
                eprintln!("unknown flag: {other}");
                eprintln!("usage: qir-core [--json] [--llvm] [file.qasm]");
                std::process::exit(2);
            }
        }
    }

    let source = match file_path {
        Some(path) => std::fs::read_to_string(&path)
            .unwrap_or_else(|e| { eprintln!("error reading {path}: {e}"); std::process::exit(1); }),
        None => {
            let mut buf = String::new();
            if io::stdin().read_to_string(&mut buf).is_err() {
                eprintln!("error reading stdin");
                std::process::exit(1);
            }
            buf
        }
    };

    let circuit = match qir_core::parse(&source) {
        Ok(c) => c,
        Err(e) => { eprintln!("parse error: {e}"); std::process::exit(1); }
    };

    let native = match qir_core::transpile_to_native(&circuit) {
        Ok(n) => n,
        Err(e) => { eprintln!("transpile error: {e}"); std::process::exit(1); }
    };

    if json_mode {
        let ir = match qir_core::emit_llvm_ir(&native) {
            Ok(ir) => ir,
            Err(e) => { eprintln!("lowering error: {e}"); std::process::exit(1); }
        };
        let analysis = qir_core::analyze_circuit(&native);
        let mut counts = BTreeMap::<String, usize>::new();
        for gate in &native.gates {
            let name = if gate.name == "cnot" { "cx" } else { gate.name.as_str() };
            *counts.entry(name.to_string()).or_insert(0) += 1;
        }
        let count_json = counts
            .iter()
            .map(|(name, count)| format!("\"{}\":{}", json_escape(name), count))
            .collect::<Vec<_>>()
            .join(",");
        let set_json = counts
            .keys()
            .map(|name| format!("\"{}\"", json_escape(name)))
            .collect::<Vec<_>>()
            .join(",");
        println!(
            "{{\"ok\":true,\"num_qubits\":{},\"instruction_count\":{},\"qubit_allocations\":{},\"gate_depth\":{},\"single_qubit_gates\":{},\"two_qubit_gates\":{},\"multi_qubit_gates\":{},\"total_gates\":{},\"native_gate_count\":{{{}}},\"native_gate_set\":[{}],\"qir\":\"{}\"}}",
            native.num_qubits,
            analysis.instruction_count,
            analysis.qubit_allocations,
            analysis.gate_depth,
            analysis.single_qubit_gates,
            analysis.two_qubit_gates,
            analysis.multi_qubit_gates,
            analysis.total_gates,
            count_json,
            set_json,
            json_escape(&ir),
        );
    } else {
        match qir_core::emit_llvm_ir(&native) {
            Ok(ir) => print!("{ir}"),
            Err(e) => { eprintln!("lowering error: {e}"); std::process::exit(1); }
        }
    }
}


fn json_escape(value: &str) -> String {
    let mut out = String::with_capacity(value.len() + 16);
    for ch in value.chars() {
        match ch {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if (c as u32) < 0x20 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out
}
