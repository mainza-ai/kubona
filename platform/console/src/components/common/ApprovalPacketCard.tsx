'use client';

import React, { useState } from 'react';
import { ShieldCheck, Check, X, AlertTriangle, ExternalLink, Clock } from 'lucide-react';

import type { ApprovalPacketView } from '@/types';

export type { ApprovalPacketView } from '@/types';

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
    <div className="w-full bg-surface border border-warning-edge rounded-sheet p-6 shadow-[var(--shadow-float)] space-y-4">
      <div className="flex items-center justify-between">
        <span className="text-label font-semibold text-text flex items-center space-x-2">
          <ShieldCheck className="w-4 h-4 text-warning-ink" />
          <span>Approval Packet</span>
        </span>
        <span className="text-[12px] font-mono text-warning-ink bg-warning-wash px-2.5 py-0.5 rounded-capsule border border-warning-edge">
          ≤ ${typeof packet.costMaxUsd === 'number' && Number.isFinite(packet.costMaxUsd) ? packet.costMaxUsd.toFixed(2) : '—'} · ${typeof packet.budgetRemainingUsd === 'number' && Number.isFinite(packet.budgetRemainingUsd) ? packet.budgetRemainingUsd.toFixed(2) : '—'} left{packet.budgetBreached ? ' · OVERSPENT' : ''}
        </span>
      </div>
      {packet.budgetBreached && (
        <div className="p-2.5 rounded-row bg-danger-wash border border-danger-edge text-danger-ink text-[12px] font-code text-center">
          Budget breached — ceiling exceeded. Authorization must refuse until funded.
        </div>
      )}

      <div className="grid grid-cols-2 gap-3 text-label">
        <div>
          <span className="text-muted block text-[12px]">Backend</span>
          <span className="font-mono text-info-ink">{packet.backend}</span>
        </div>
        <div>
          <span className="text-muted block text-[12px]">Program</span>
          <span className="font-mono text-text">{packet.programHash}</span>
        </div>
        <div>
          <span className="text-muted block text-[12px]">Qubits × Shots</span>
          <span className="font-mono text-text">
            {packet.qubits} × {packet.shots.toLocaleString()}
          </span>
        </div>
        <div>
          <span className="text-muted block text-[12px]">Cost basis</span>
          <span className="text-label-secondary">{packet.costBasis}</span>
        </div>
        <div>
          <span className="text-muted block text-[12px]">Blast radius</span>
          <span className="text-label-secondary">{packet.blastRadius}</span>
        </div>
        <div>
          <span className="text-muted block text-[12px]">Reversibility</span>
          <span className="text-label-secondary">{packet.reversibility}</span>
        </div>
      </div>

      {(packet.evidence.dryRun || packet.evidence.classicalAnchor) && (
        <div className="p-3 rounded-row bg-info-wash border border-info-edge text-[12px] text-info-ink space-y-1">
          {packet.evidence.dryRun && <div>Dry run: {packet.evidence.dryRun}</div>}
          {packet.evidence.classicalAnchor && <div>Anchor: {packet.evidence.classicalAnchor}</div>}
        </div>
      )}

      {(packet.alternatives.simFirstSavings || packet.alternatives.fewerShotsSavings) && (
        <div className="p-3 rounded-row bg-card border border-border text-[12px] text-label-secondary space-y-1">
          {packet.alternatives.simFirstSavings && (
            <div>Sim first: {packet.alternatives.simFirstSavings}</div>
          )}
          {packet.alternatives.fewerShotsSavings && (
            <div>Fewer shots: {packet.alternatives.fewerShotsSavings}</div>
          )}
        </div>
      )}

      <div className="flex items-center justify-between text-[12px] font-mono text-muted">
        <span className="flex items-center space-x-1">
          <Clock className="w-3 h-3" />
          <span>Expires {packet.expiresAtIso}</span>
        </span>
        <span>
          {packet.packetId} · nonce {packet.noncePreview} · {packet.agentId}
        </span>
      </div>

      {decisionError && (
        <div className="p-3 rounded-row bg-danger-wash border border-danger-edge text-danger-ink text-label flex items-center space-x-2">
          <AlertTriangle className="w-4 h-4 shrink-0" />
          <span>{decisionError}</span>
        </div>
      )}

      {!showDeny ? (
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={onApproveOnce}
            disabled={isSubmitting || Boolean(packet.budgetBreached)}
            className="flex-1 py-2.5 px-4 text-label font-semibold rounded-row flex items-center justify-center space-x-2 transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] bg-gradient-to-r from-emerald-600 to-teal-600 hover:from-emerald-500 hover:to-teal-500 text-on-accent shadow-lg active:scale-[0.98] disabled:opacity-50"
          >
            <Check className="w-4 h-4" />
            <span>{isSubmitting ? 'Dispatching…' : 'Approve once'}</span>
          </button>
          <button
            onClick={onApproveUpToMax}
            disabled={isSubmitting || Boolean(packet.budgetBreached)}
            className="py-2.5 px-4 bg-fill hover:bg-fill-hover border border-border text-label-secondary text-label font-medium rounded-row transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] disabled:opacity-50"
          >
            {costMaxLabel != null ? `Approve up to ${costMaxLabel}` : 'Approve up to max'}
          </button>
          <button
            onClick={() => setShowDeny(true)}
            disabled={isSubmitting}
            className="py-2.5 px-4 bg-fill hover:bg-danger-wash border border-border text-danger-ink text-label font-medium rounded-row transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] flex items-center space-x-1.5 disabled:opacity-50"
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
            className="w-full px-3 py-2 rounded-row bg-field border border-danger-edge text-label text-text placeholder:text-faint focus:outline-none focus:border-danger-edge"
          />
          <div className="flex items-center gap-2">
            <button
              onClick={() => denyReason.trim() && onDeny(denyReason.trim())}
              disabled={isSubmitting || !denyReason.trim()}
              className="flex-1 py-2 px-4 rounded-row bg-rose-600 hover:bg-rose-500 text-on-accent text-label font-semibold transition disabled:opacity-50"
            >
              Confirm denial
            </button>
            <button
              onClick={() => {
                setShowDeny(false);
                setDenyReason('');
              }}
              className="py-2 px-4 rounded-row bg-fill hover:bg-fill-hover text-label-secondary text-label transition"
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
              className="flex items-center space-x-1 text-accent-ink hover:underline"
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
              className="flex items-center space-x-1 text-accent-ink hover:underline"
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
