//! qir-core CLI: read QASM from a file or stdin, emit QIR LLVM IR or JSON metrics.
//!
//! Usage: qir-core [--json] [--llvm] [file.qasm]   (reads stdin when no file given)

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

    let native = qir_core::transpile_to_native(&circuit);

    if json_mode {
        if let Err(e) = qir_core::emit_llvm_ir(&native) {
            eprintln!("lowering error: {e}");
            std::process::exit(1);
        }
        let summary = qir_core::analyze_circuit(&native);
        println!("{}", summary.to_json());
    } else {
        match qir_core::emit_llvm_ir(&native) {
            Ok(ir) => print!("{ir}"),
            Err(e) => { eprintln!("lowering error: {e}"); std::process::exit(1); }
        }
    }
}
