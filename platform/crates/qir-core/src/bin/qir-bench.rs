//! Repeatable local benchmark for the QIR lowering path.
//!
//! Run: cargo run --release --bin qir-bench
//! This is trend evidence for the current host, not a release budget.

use std::hint::black_box;
use std::time::Instant;

fn qasm(gates: usize, qubits: usize) -> String {
    let mut out = String::with_capacity(gates * 24 + 128);
    out.push_str("OPENQASM 2.0;\ninclude \"qelib1.inc\";\n");
    out.push_str(&format!("qreg q[{qubits}];\ncreg c[{qubits}];\n"));
    for i in 0..gates {
        let q = i % qubits;
        match i % 5 {
            0 => out.push_str(&format!("h q[{q}];\n")),
            1 => out.push_str(&format!("rz({:.6}) q[{q}];\n", (i as f64) * 0.01)),
            2 => out.push_str(&format!("ry({:.6}) q[{q}];\n", (i as f64) * 0.02)),
            3 if qubits > 1 => {
                let t = (q + 1) % qubits;
                out.push_str(&format!("cx q[{q}],q[{t}];\n"));
            }
            _ => out.push_str(&format!("x q[{q}];\n")),
        }
    }
    for q in 0..qubits {
        out.push_str(&format!("measure q[{q}] -> c[{q}];\n"));
    }
    out
}

fn percentile(sorted: &[f64], p: f64) -> f64 {
    if sorted.is_empty() {
        return 0.0;
    }
    let index = ((sorted.len() as f64 - 1.0) * p).round() as usize;
    sorted[index.min(sorted.len() - 1)]
}

fn main() {
    let cases = [(2usize, 2usize), (10, 4), (100, 8), (1_000, 12), (10_000, 16)];
    for (gates, qubits) in cases {
        let source = qasm(gates, qubits);
        let iterations = if gates >= 10_000 { 20 } else if gates >= 1_000 { 50 } else { 200 };
        let mut parse = Vec::with_capacity(iterations);
        let mut transpile = Vec::with_capacity(iterations);
        let mut emit = Vec::with_capacity(iterations);
        for _ in 0..3 {
            let circuit = qir_core::parse(&source).expect("bench qasm must parse");
            let native = qir_core::transpile_to_native(&circuit).expect("transpile");
            black_box(qir_core::emit_llvm_ir(&native).expect("emit"));
        }
        for _ in 0..iterations {
            let start = Instant::now();
            let circuit = qir_core::parse(&source).expect("bench qasm must parse");
            parse.push(start.elapsed().as_secs_f64() * 1e6);

            let start = Instant::now();
            let native = qir_core::transpile_to_native(&circuit).expect("transpile");
            transpile.push(start.elapsed().as_secs_f64() * 1e6);

            let start = Instant::now();
            let ir = qir_core::emit_llvm_ir(&native).expect("emit");
            emit.push(start.elapsed().as_secs_f64() * 1e6);
            black_box(ir);
        }
        parse.sort_by(f64::total_cmp);
        transpile.sort_by(f64::total_cmp);
        emit.sort_by(f64::total_cmp);
        println!(
            "{{\"gates\":{gates},\"qubits\":{qubits},\"iterations\":{iterations},\"qasm_bytes\":{},\"parse_p50_us\":{:.2},\"parse_p95_us\":{:.2},\"transpile_p50_us\":{:.2},\"transpile_p95_us\":{:.2},\"emit_p50_us\":{:.2},\"emit_p95_us\":{:.2}}}",
            source.len(),
            percentile(&parse, 0.50),
            percentile(&parse, 0.95),
            percentile(&transpile, 0.50),
            percentile(&transpile, 0.95),
            percentile(&emit, 0.50),
            percentile(&emit, 0.95),
        );
    }
}
