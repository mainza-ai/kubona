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
    <div data-provenance={hasReceipt ? 'measured' : undefined} className="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 text-[var(--color-text)] space-y-4">
      <div className="flex items-center justify-between border-b border-[var(--color-border)] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-[var(--color-info-wash)] border border-[var(--color-info-edge)] text-[var(--color-info-ink)]">
            <Cpu className="w-5 h-5" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-semibold text-[var(--color-text)]">Post-QIR Quantum Circuit</h3>
              {hasReceipt && <ProvenanceBadge kind="measured" detail="Backend compiler receipt." />}
            </div>
            <p className="text-xs text-[var(--color-muted)]">
              {displayQubits != null ? displayQubits : '—'} Qubits • Depth {displayDepth != null ? displayDepth : '—'}
            </p>
          </div>
        </div>
      </div>

      {!hasReceipt ? (
        <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-card)] p-5 text-center space-y-1.5">
          <p className="text-xs font-medium text-[var(--color-text)]">No post-QIR synthesis receipt is recorded for this campaign.</p>
          <p className="text-[12px] text-[var(--color-muted)]">
            The editor appears only after the backend returns compiler QIR; no schematic circuit is substituted.
          </p>
        </div>
      ) : (
        <>
          <div className="grid grid-cols-4 gap-3 text-xs">
            <div className="p-2.5 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)]">
              <span className="text-[var(--color-muted)] block text-[12px]">T-Count</span>
              <span data-metric className="font-mono text-base font-semibold text-[var(--color-warning-ink)]">{displayTCount ?? '—'}</span>
            </div>
            <div className="p-2.5 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)]">
              <span className="text-[var(--color-muted)] block text-[12px]">T-Depth</span>
              <span data-metric className="font-mono text-base font-semibold text-[var(--color-success-ink)]">{displayTDepth ?? '—'}</span>
            </div>
            <div className="p-2.5 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)]">
              <span className="text-[var(--color-muted)] block text-[12px]">Qubits</span>
              <span data-metric className="font-mono text-base font-semibold text-[var(--color-info-ink)]">{displayQubits ?? '—'}</span>
            </div>
            <div className="p-2.5 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)]">
              <span className="text-[var(--color-muted)] block text-[12px]">Depth</span>
              <span data-metric className="font-mono text-base font-semibold text-[var(--color-soft-lilac-ink)]">{displayDepth ?? '—'}</span>
            </div>
          </div>
          <div className="relative rounded-xl border border-[var(--color-border)] bg-[var(--color-field)] p-4 font-mono text-xs text-[var(--color-text)] overflow-x-auto h-64 scrollbar-thin">
            <button
              onClick={copyQir}
              aria-label={copied ? 'QIR copied to clipboard' : 'Copy QIR to clipboard'}
              className="absolute top-3 right-3 p-1.5 rounded-lg bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] transition text-[var(--color-label-secondary)]"
            >
              {copied ? <Check className="w-4 h-4 text-[var(--color-success-ink)]" /> : <Copy className="w-4 h-4" />}
            </button>
            <pre className="text-[var(--color-info-ink)] leading-relaxed">{qirCode}</pre>
          </div>
        </>
      )}
    </div>
  );
}
