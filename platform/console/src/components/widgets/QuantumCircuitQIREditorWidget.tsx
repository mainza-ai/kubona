'use client';

import React, { useState } from 'react';
import { Cpu, Copy, Check } from 'lucide-react';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';

interface QuantumCircuitQIREditorWidgetProps {
  qubits?: number;
  circuitDepth?: number;
  tCount?: number;
  tDepth?: number;
  qirCode?: string;
}

function countTInstructions(qir: string): number {
  const matches = qir.match(/__quantum__qis__t__body/g);
  return matches ? matches.length : 0;
}

/**
 * Displays a post-QIR synthesis receipt. Without a receipt there is no
 * circuit body to show: the old schematic, shot-overhead, and cut figures
 * were illustrative, not session data.
 */
export function QuantumCircuitQIREditorWidget({
  qubits,
  circuitDepth,
  tCount,
  tDepth,
  qirCode,
}: QuantumCircuitQIREditorWidgetProps) {
  const [copied, setCopied] = useState(false);
  const hasReceipt = typeof qirCode === 'string' && qirCode.trim() !== '';
  const displayQubits = typeof qubits === 'number' && Number.isFinite(qubits) ? qubits : null;
  const displayDepth = typeof circuitDepth === 'number' && Number.isFinite(circuitDepth) ? circuitDepth : null;
  const displayTCount = typeof tCount === 'number' && Number.isFinite(tCount)
    ? tCount
    : (hasReceipt ? countTInstructions(qirCode) : null);
  const displayTDepth = typeof tDepth === 'number' && Number.isFinite(tDepth) ? tDepth : null;

  const copyQir = () => {
    if (!hasReceipt) return;
    navigator.clipboard.writeText(qirCode).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div data-provenance={hasReceipt ? 'measured' : undefined} className="rounded-2xl border border-white/[0.08] bg-black/60 p-5 text-white space-y-4">
      <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-cyan-500/10 border border-cyan-500/20 text-cyan-400">
            <Cpu className="w-5 h-5" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-semibold text-white/90">Post-QIR Quantum Circuit</h3>
              {hasReceipt && <ProvenanceBadge kind="measured" detail="Backend compiler receipt." />}
            </div>
            <p className="text-xs text-white/50">
              {displayQubits != null ? displayQubits : '—'} Qubits • Depth {displayDepth != null ? displayDepth : '—'}
            </p>
          </div>
        </div>
      </div>

      {!hasReceipt ? (
        <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-5 text-center space-y-1.5">
          <p className="text-xs font-medium text-white/80">No post-QIR synthesis receipt is recorded for this campaign.</p>
          <p className="text-[12px] text-slate-400">
            The editor appears only after the backend returns compiler QIR; no schematic circuit is substituted.
          </p>
        </div>
      ) : (
        <>
          <div className="grid grid-cols-4 gap-3 text-xs">
            <div className="p-2.5 rounded-xl bg-white/[0.02] border border-white/[0.06]">
              <span className="text-white/40 block text-[12px]">T-Count</span>
              <span data-metric className="font-mono text-base font-semibold text-amber-400">{displayTCount ?? '—'}</span>
            </div>
            <div className="p-2.5 rounded-xl bg-white/[0.02] border border-white/[0.06]">
              <span className="text-white/40 block text-[12px]">T-Depth</span>
              <span data-metric className="font-mono text-base font-semibold text-emerald-400">{displayTDepth ?? '—'}</span>
            </div>
            <div className="p-2.5 rounded-xl bg-white/[0.02] border border-white/[0.06]">
              <span className="text-white/40 block text-[12px]">Qubits</span>
              <span data-metric className="font-mono text-base font-semibold text-cyan-400">{displayQubits ?? '—'}</span>
            </div>
            <div className="p-2.5 rounded-xl bg-white/[0.02] border border-white/[0.06]">
              <span className="text-white/40 block text-[12px]">Depth</span>
              <span data-metric className="font-mono text-base font-semibold text-purple-400">{displayDepth ?? '—'}</span>
            </div>
          </div>
          <div className="relative rounded-xl border border-white/[0.06] bg-black/80 p-4 font-mono text-xs text-white/80 overflow-x-auto h-64 scrollbar-thin">
            <button
              onClick={copyQir}
              aria-label={copied ? 'QIR copied to clipboard' : 'Copy QIR to clipboard'}
              className="absolute top-3 right-3 p-1.5 rounded-lg bg-white/10 hover:bg-white/20 transition text-white/70"
            >
              {copied ? <Check className="w-4 h-4 text-emerald-400" /> : <Copy className="w-4 h-4" />}
            </button>
            <pre className="text-cyan-300/90 leading-relaxed">{qirCode}</pre>
          </div>
        </>
      )}
    </div>
  );
}
