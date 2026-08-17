//! qir-core CLI: read QASM from a file or stdin, emit QIR-like IR.
//!
//! Usage: qir-core [file.qasm]   (reads stdin when no file given)

use std::io::{self, Read};

fn main() {
    let source = match std::env::args().nth(1) {
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
    print!("{}", qir_core::emit_qir(&native));
}
