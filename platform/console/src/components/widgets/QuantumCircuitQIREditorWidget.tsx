'use client';

import React, { useState } from 'react';
import { Cpu, Scissors,   Check, Copy } from 'lucide-react';

interface QuantumCircuitQIREditorWidgetProps {
  qubits?: number;
  circuitDepth?: number;
  tCount?: number;
  tDepth?: number;
  qirCode?: string;
}

export function QuantumCircuitQIREditorWidget({
  qubits,
  circuitDepth,
  tCount = 8,
  tDepth = 4,
  qirCode,
}: QuantumCircuitQIREditorWidgetProps) {
  const [mode, setMode] = useState<'schematic' | 'qir' | 'cutting'>('schematic');
  const [wireCutSelected, setWireCutSelected] = useState<number>(2);
  const [copied, setCopied] = useState(false);
  // Live synthesis receipt required: without qirCode the body is schematic
  // only. Labels (qubits/depth) render live when bound, body stays watermarked.
  const hasReceipt = qirCode != null && qirCode.trim() !== '';
  const displayQubits = typeof qubits === 'number' && Number.isFinite(qubits) ? qubits : null;
  const displayDepth = typeof circuitDepth === 'number' && Number.isFinite(circuitDepth) ? circuitDepth : null;

  const sampleQir = qirCode || `; schematic — illustrative, not session data (no live synthesis receipt)
; ModuleID = 'kubona_qksd_circuit'
source_filename = "kubona_qir_v1"
target datalayout = "e-m:e-p:64:64-i64:64-f80:128-n8:16:32:64-S128"

%Qubit = type opaque
%Result = type opaque

declare void @__quantum__qis__h__body(%Qubit*)
declare void @__quantum__qis__cnot__body(%Qubit*, %Qubit*)
declare void @__quantum__qis__t__body(%Qubit*)
declare void @__quantum__qis__rz__body(double, %Qubit*)
declare void @__quantum__qis__mz__body(%Qubit*, %Result*)

define void @main() #0 {
entry:
  call void @__quantum__qis__h__body(%Qubit* null)
  call void @__quantum__qis__cnot__body(%Qubit* null, %Qubit* nonnull inttoptr (i64 1 to %Qubit*))
  call void @__quantum__qis__t__body(%Qubit* null)
  call void @__quantum__qis__rz__body(double 0.785398, %Qubit* nonnull inttoptr (i64 1 to %Qubit*))
  ret void
}`;

  const copyQir = () => {
    navigator.clipboard.writeText(sampleQir).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/60 p-5 backdrop-blur-xl text-white space-y-4">
      {!hasReceipt && (
        <div className="p-2.5 rounded-xl bg-amber-950/50 border border-amber-500/40 text-amber-200 text-[11px] font-code text-center">
          synthetic demo — not session data. No live synthesis receipt; schematic body is illustrative.
        </div>
      )}
      {/* Header */}
      <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-cyan-500/10 border border-cyan-500/20 text-cyan-400">
            <Cpu className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white/90">Post-QIR Quantum Circuit Editor (Preview)<span className="ml-2 align-middle text-[9px] font-mono uppercase px-2 py-0.5 rounded-full bg-white/[0.04] border border-white/10 text-slate-400">Illustrative</span></h3>
            <p className="text-xs text-white/50">{displayQubits != null ? displayQubits : '—'} Qubits • Depth {displayDepth != null ? displayDepth : '—'} • Family 279 Clifford+T Synthesis {!hasReceipt ? '· schematic only' : ''}</p>
          </div>
        </div>
        <div className="flex items-center space-x-1.5 text-xs">
          <button
            onClick={() => setMode('schematic')}
            className={`px-2.5 py-1 rounded-lg border transition ${
              mode === 'schematic'
                ? 'bg-cyan-600/20 border-cyan-500/40 text-cyan-300'
                : 'border-white/[0.06] text-white/60 hover:text-white'
            }`}
          >
            Schematic
          </button>
          <button
            onClick={() => setMode('qir')}
            className={`px-2.5 py-1 rounded-lg border transition ${
              mode === 'qir'
                ? 'bg-cyan-600/20 border-cyan-500/40 text-cyan-300'
                : 'border-white/[0.06] text-white/60 hover:text-white'
            }`}
          >
            QIR LLVM
          </button>
          <button
            onClick={() => setMode('cutting')}
            className={`px-2.5 py-1 rounded-lg border transition ${
              mode === 'cutting'
                ? 'bg-cyan-600/20 border-cyan-500/40 text-cyan-300'
                : 'border-white/[0.06] text-white/60 hover:text-white'
            }`}
          >
            Wire Cuts
          </button>
        </div>
      </div>

      {/* Metrics Row */}
      <div className="grid grid-cols-4 gap-3 text-xs">
        <div className="p-2.5 rounded-xl bg-white/[0.02] border border-white/[0.06]">
          <span className="text-white/40 block text-[10px]">T-Count (Non-Clifford)</span>
          <span className="font-mono text-base font-semibold text-amber-400">{tCount}</span>
        </div>
        <div className="p-2.5 rounded-xl bg-white/[0.02] border border-white/[0.06]">
          <span className="text-white/40 block text-[10px]">T-Depth (Family 279)</span>
          <span className="font-mono text-base font-semibold text-emerald-400">{tDepth}</span>
        </div>
        <div className="p-2.5 rounded-xl bg-white/[0.02] border border-white/[0.06]">
          <span className="text-white/40 block text-[10px]">Logical Qubit Overhead</span>
          <span className="font-mono text-base font-semibold text-cyan-400">12 : 288 (BB Code)</span>
        </div>
        <div className="p-2.5 rounded-xl bg-white/[0.02] border border-white/[0.06]">
          <span className="text-white/40 block text-[10px]">LOCC Sampling Cost</span>
          <span className="font-mono text-base font-semibold text-purple-400">4.2x Shots</span>
        </div>
      </div>

      {/* Main View Area */}
      {mode === 'schematic' && (
        <div className="p-4 rounded-xl border border-white/[0.06] bg-white/[0.02] overflow-x-auto">
          <div className="space-y-4 min-w-[500px]">
            {Array.from({ length: displayQubits ?? 4 }).map((_, qIdx) => (
              <div key={qIdx} className="flex items-center space-x-3">
                <span className="w-10 text-xs font-mono text-white/60">q[{qIdx}]</span>
                <div className="flex-1 flex items-center relative h-8">
                  {/* Wire line */}
                  <div className="absolute left-0 right-0 h-0.5 bg-white/20" />
                  {/* Wire cut indicator */}
                  {qIdx === 1 && (
                    <div className="absolute left-[65%] -top-2 flex flex-col items-center z-10">
                      <Scissors className="w-3.5 h-3.5 text-rose-400" />
                      <span className="text-[9px] font-mono text-rose-400 bg-black/80 px-1 rounded">Cut A</span>
                    </div>
                  )}
                  {/* Gates */}
                  <div className="flex items-center space-x-6 relative z-0 pl-4">
                    {qIdx === 0 && (
                      <>
                        <div className="px-2.5 py-1 rounded bg-blue-500/20 border border-blue-400 text-blue-300 font-mono text-xs font-bold shadow">
                          H
                        </div>
                        <div className="w-4 h-4 rounded-full bg-cyan-400 border border-white" />
                        <div className="px-2.5 py-1 rounded bg-amber-500/20 border border-amber-400 text-amber-300 font-mono text-xs font-bold shadow">
                          T
                        </div>
                      </>
                    )}
                    {qIdx === 1 && (
                      <>
                        <div className="w-8" />
                        <div className="w-6 h-6 rounded-full border-2 border-cyan-400 flex items-center justify-center text-cyan-400 font-bold text-xs">
                          +
                        </div>
                        <div className="px-2 py-1 rounded bg-purple-500/20 border border-purple-400 text-purple-300 font-mono text-xs shadow">
                          Rz(π/4)
                        </div>
                      </>
                    )}
                    {qIdx === 2 && (
                      <>
                        <div className="px-2.5 py-1 rounded bg-amber-500/20 border border-amber-400 text-amber-300 font-mono text-xs font-bold shadow">
                          T
                        </div>
                        <div className="w-8" />
                        <div className="w-4 h-4 rounded-full bg-cyan-400 border border-white" />
                      </>
                    )}
                    {qIdx === 3 && (
                      <>
                        <div className="px-2.5 py-1 rounded bg-blue-500/20 border border-blue-400 text-blue-300 font-mono text-xs font-bold shadow">
                          H
                        </div>
                        <div className="w-8" />
                        <div className="w-6 h-6 rounded-full border-2 border-cyan-400 flex items-center justify-center text-cyan-400 font-bold text-xs">
                          +
                        </div>
                      </>
                    )}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {mode === 'qir' && (
        <div className="relative rounded-xl border border-white/[0.06] bg-black/80 p-4 font-mono text-xs text-white/80 overflow-x-auto h-64 scrollbar-thin">
          <button
            onClick={copyQir}
            aria-label={copied ? 'QIR copied to clipboard' : 'Copy QIR to clipboard'}
            className="absolute top-3 right-3 p-1.5 rounded-lg bg-white/10 hover:bg-white/20 transition text-white/70"
          >
            {copied ? <Check className="w-4 h-4 text-emerald-400" /> : <Copy className="w-4 h-4" />}
          </button>
          <pre className="text-cyan-300/90 leading-relaxed">{sampleQir}</pre>
        </div>
      )}

      {mode === 'cutting' && (
        <div className="p-4 rounded-xl border border-white/[0.06] bg-white/[0.02] space-y-3 text-xs">
          <div className="flex justify-between items-center text-white/80">
            <span>Automatic Wire Cutting Partitioning (LOCC)</span>
            <span className="text-rose-400 font-mono">Cut Overhead: κ = 4.2</span>
          </div>
          <p className="text-white/50">
            Partitions the 4-qubit circuit into two 2-qubit subcircuits executable on narrow QPUs without SWAP chains.
          </p>
          <div className="flex items-center space-x-2 pt-2">
            <button
              onClick={() => setWireCutSelected(1)}
              className={`px-3 py-1.5 rounded-lg border font-mono transition ${
                wireCutSelected === 1
                  ? 'bg-rose-500/20 border-rose-500/40 text-rose-300'
                  : 'border-white/[0.08] text-white/60'
              }`}
            >
              1 Cut (Subcircuits: 2x2)
            </button>
            <button
              onClick={() => setWireCutSelected(2)}
              className={`px-3 py-1.5 rounded-lg border font-mono transition ${
                wireCutSelected === 2
                  ? 'bg-rose-500/20 border-rose-500/40 text-rose-300'
                  : 'border-white/[0.08] text-white/60'
              }`}
            >
              2 Cuts (Subcircuits: 3x1)
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
