'use client';

import React, { useState } from 'react';
import { ShieldCheck, Check, X, AlertTriangle, ExternalLink, Clock } from 'lucide-react';

export interface ApprovalPacketView {
  packetId: string;
  agentId: string;
  backend: string;
  programHash: string;
  qubits: number;
  shots: number;
  costMaxUsd: number;
  budgetRemainingUsd: number;
  costBasis: string;
  blastRadius: string;
  reversibility: string;
  evidence: {
    dryRun?: string;
    classicalAnchor?: string;
    quoteUrl?: string;
    notebookUrl?: string;
  };
  alternatives: {
    simFirstSavings?: string;
    fewerShotsSavings?: string;
  };
  expiresAtIso: string;
  noncePreview: string;
  // Fail-closed overspend flag: true when ceiling - spent < 0 at build time.
  // Renderers must surface this as a breach, never clamp silently.
  budgetBreached?: boolean;
}

interface ApprovalPacketCardProps {
  packet: ApprovalPacketView;
  isSubmitting?: boolean;
  decisionError?: string | null;
  onApproveOnce: () => void;
  onApproveUpToMax: () => void;
  onDeny: (reason: string) => void;
}

/**
 * Burden-scaled approval card (Q-11): everything a competent approver needs
 * to reconstruct the decision later — exact action, cost bound + budget left,
 * blast radius, reversibility, evidence links, alternatives, expiry — with
 * single-use decision semantics (approve-once / up-to-max / deny+reason).
 * Denial requires a reason, which is returned to the agent.
 */
export function ApprovalPacketCard({
  packet,
  isSubmitting = false,
  decisionError = null,
  onApproveOnce,
  onApproveUpToMax,
  onDeny,
}: ApprovalPacketCardProps) {
  const [denyReason, setDenyReason] = useState('');
  const [showDeny, setShowDeny] = useState(false);
  // Wave-2 money copy: the up-to-max button restates the exact dollar bound
  // (2dp, already in scope) — no invented figure; unpriced falls back.
  const costMaxLabel =
    typeof packet.costMaxUsd === 'number' && Number.isFinite(packet.costMaxUsd)
      ? `$${packet.costMaxUsd.toFixed(2)}`
      : null;

  return (
    <div className="w-full bg-[var(--color-surface)] border border-[var(--color-warning-edge)] rounded-3xl p-6 shadow-[var(--shadow-float)] space-y-4">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-[var(--color-text)] flex items-center space-x-2">
          <ShieldCheck className="w-4 h-4 text-[var(--color-warning-ink)]" />
          <span>Approval Packet</span>
        </span>
        <span className="text-[12px] font-mono text-[var(--color-warning-ink)] bg-[var(--color-warning-wash)] px-2.5 py-0.5 rounded-full border border-[var(--color-warning-edge)]">
          ≤ ${typeof packet.costMaxUsd === 'number' && Number.isFinite(packet.costMaxUsd) ? packet.costMaxUsd.toFixed(2) : '—'} · ${typeof packet.budgetRemainingUsd === 'number' && Number.isFinite(packet.budgetRemainingUsd) ? packet.budgetRemainingUsd.toFixed(2) : '—'} left{packet.budgetBreached ? ' · OVERSPENT' : ''}
        </span>
      </div>
      {packet.budgetBreached && (
        <div className="p-2.5 rounded-xl bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-[12px] font-code text-center">
          Budget breached — ceiling exceeded. Authorization must refuse until funded.
        </div>
      )}

      <div className="grid grid-cols-2 gap-3 text-xs">
        <div>
          <span className="text-[var(--color-muted)] block text-[12px]">Backend</span>
          <span className="font-mono text-[var(--color-info-ink)]">{packet.backend}</span>
        </div>
        <div>
          <span className="text-[var(--color-muted)] block text-[12px]">Program</span>
          <span className="font-mono text-[var(--color-text)]">{packet.programHash}</span>
        </div>
        <div>
          <span className="text-[var(--color-muted)] block text-[12px]">Qubits × Shots</span>
          <span className="font-mono text-[var(--color-text)]">
            {packet.qubits} × {packet.shots.toLocaleString()}
          </span>
        </div>
        <div>
          <span className="text-[var(--color-muted)] block text-[12px]">Cost basis</span>
          <span className="text-[var(--color-label-secondary)]">{packet.costBasis}</span>
        </div>
        <div>
          <span className="text-[var(--color-muted)] block text-[12px]">Blast radius</span>
          <span className="text-[var(--color-label-secondary)]">{packet.blastRadius}</span>
        </div>
        <div>
          <span className="text-[var(--color-muted)] block text-[12px]">Reversibility</span>
          <span className="text-[var(--color-label-secondary)]">{packet.reversibility}</span>
        </div>
      </div>

      {(packet.evidence.dryRun || packet.evidence.classicalAnchor) && (
        <div className="p-3 rounded-xl bg-[var(--color-info-wash)] border border-[var(--color-info-edge)] text-[12px] text-[var(--color-info-ink)] space-y-1">
          {packet.evidence.dryRun && <div>Dry run: {packet.evidence.dryRun}</div>}
          {packet.evidence.classicalAnchor && <div>Anchor: {packet.evidence.classicalAnchor}</div>}
        </div>
      )}

      {(packet.alternatives.simFirstSavings || packet.alternatives.fewerShotsSavings) && (
        <div className="p-3 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)] text-[12px] text-[var(--color-label-secondary)] space-y-1">
          {packet.alternatives.simFirstSavings && (
            <div>Sim first: {packet.alternatives.simFirstSavings}</div>
          )}
          {packet.alternatives.fewerShotsSavings && (
            <div>Fewer shots: {packet.alternatives.fewerShotsSavings}</div>
          )}
        </div>
      )}

      <div className="flex items-center justify-between text-[12px] font-mono text-[var(--color-muted)]">
        <span className="flex items-center space-x-1">
          <Clock className="w-3 h-3" />
          <span>Expires {packet.expiresAtIso}</span>
        </span>
        <span>
          {packet.packetId} · nonce {packet.noncePreview} · {packet.agentId}
        </span>
      </div>

      {decisionError && (
        <div className="p-3 rounded-xl bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-xs flex items-center space-x-2">
          <AlertTriangle className="w-4 h-4 shrink-0" />
          <span>{decisionError}</span>
        </div>
      )}

      {!showDeny ? (
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={onApproveOnce}
            disabled={isSubmitting || Boolean(packet.budgetBreached)}
            className="flex-1 py-2.5 px-4 text-xs font-semibold rounded-xl flex items-center justify-center space-x-2 transition-all bg-gradient-to-r from-emerald-600 to-teal-600 hover:from-emerald-500 hover:to-teal-500 text-[var(--color-on-accent)] shadow-lg active:scale-[0.98] disabled:opacity-50"
          >
            <Check className="w-4 h-4" />
            <span>{isSubmitting ? 'Dispatching…' : 'Approve once'}</span>
          </button>
          <button
            onClick={onApproveUpToMax}
            disabled={isSubmitting || Boolean(packet.budgetBreached)}
            className="py-2.5 px-4 bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[var(--color-label-secondary)] text-xs font-medium rounded-xl transition-all disabled:opacity-50"
          >
            {costMaxLabel != null ? `Approve up to ${costMaxLabel}` : 'Approve up to max'}
          </button>
          <button
            onClick={() => setShowDeny(true)}
            disabled={isSubmitting}
            className="py-2.5 px-4 bg-[var(--color-fill)] hover:bg-[var(--color-danger-wash)] border border-[var(--color-border)] text-[var(--color-danger-ink)] text-xs font-medium rounded-xl transition-all flex items-center space-x-1.5 disabled:opacity-50"
          >
            <X className="w-3.5 h-3.5" />
            <span>Deny</span>
          </button>
        </div>
      ) : (
        <div className="space-y-2">
          <input
            value={denyReason}
            onChange={(e) => setDenyReason(e.target.value)}
            placeholder="Denial reason (required — returned to the agent)…"
            maxLength={280}
            className="w-full px-3 py-2 rounded-xl bg-[var(--color-field)] border border-[var(--color-danger-edge)] text-xs text-[var(--color-text)] placeholder:text-[var(--color-faint)] focus:outline-none focus:border-[var(--color-danger-edge)]"
          />
          <div className="flex items-center gap-2">
            <button
              onClick={() => denyReason.trim() && onDeny(denyReason.trim())}
              disabled={isSubmitting || !denyReason.trim()}
              className="flex-1 py-2 px-4 rounded-xl bg-rose-600 hover:bg-rose-500 text-[var(--color-on-accent)] text-xs font-semibold transition disabled:opacity-50"
            >
              Confirm denial
            </button>
            <button
              onClick={() => {
                setShowDeny(false);
                setDenyReason('');
              }}
              className="py-2 px-4 rounded-xl bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] text-[var(--color-label-secondary)] text-xs transition"
            >
              Back
            </button>
          </div>
        </div>
      )}

      {packet.evidence.notebookUrl || packet.evidence.quoteUrl ? (
        <div className="flex items-center gap-3 text-[12px]">
          {packet.evidence.notebookUrl && (
            <a
              href={packet.evidence.notebookUrl}
              target="_blank"
              rel="noreferrer"
              className="flex items-center space-x-1 text-[var(--color-accent-ink)] hover:underline"
            >
              <ExternalLink className="w-3 h-3" />
              <span>Notebook</span>
            </a>
          )}
          {packet.evidence.quoteUrl && (
            <a
              href={packet.evidence.quoteUrl}
              target="_blank"
              rel="noreferrer"
              className="flex items-center space-x-1 text-[var(--color-accent-ink)] hover:underline"
            >
              <ExternalLink className="w-3 h-3" />
              <span>Quote</span>
            </a>
          )}
        </div>
      ) : null}
    </div>
  );
}
