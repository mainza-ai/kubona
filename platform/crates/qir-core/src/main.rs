//! qir-core CLI: read QASM from a file or stdin, emit QIR LLVM IR or JSON metrics.
//!
//! Usage: qir-core [--json] [--llvm] [file.qasm]   (reads stdin when no file given)

use std::io::{self, Read};

fn main() {
    let mut json_mode = false;
    let mut file_path = None;

    for arg in std::env::args().skip(1) {
        match arg.as_str() {
            "--json" => json_mode = true,
            "--llvm" => json_mode = false,
            other if !other.starts_with("--") => file_path = Some(other.to_string()),
            _ => (),
        }
    }

    let source = match file_path {
        Some(path) => std::fs::read_to_string(&path)
            .unwrap_or_else(|e| { eprintln!("error reading {path}: {e}"); std::process::exit(1); }),
        None => {
            let mut buf = String::new();
            io::stdin().read_to_string(&mut buf).expect("read stdin");
            buf
        }
    };

    let circuit = match qir_core::parse(&source) {
        Ok(c) => c,
        Err(e) => { eprintln!("parse error: {e}"); std::process::exit(1); }
    };

    let native = qir_core::transpile_to_native(&circuit);

    if json_mode {
        let summary = qir_core::analyze_circuit(&native);
        println!("{}", summary.to_json());
    } else {
        print!("{}", qir_core::emit_llvm_ir(&native));
    }
}
