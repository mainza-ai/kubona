"""QIR/MLIR lowering — Python reference pipeline (Phase 1).

The production core is Rust (see crates/qir-core); this reference implementation
fixes the semantics: QASM 3 subset -> canonical IR -> QIR-like LLVM text, plus a
transpile pass decomposing high-level gates to a native gate set.
"""

from .pipeline import lower, transpile_to_native

__all__ = ["lower", "transpile_to_native"]
