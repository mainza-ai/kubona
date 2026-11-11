'use client';

import React, { useState } from 'react';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';
import { KeyRound,  CheckCircle2, XCircle, ArrowDownCircle } from 'lucide-react';
import { authorizeSpend } from '@/lib/authorizeSpend';

interface CryptographicMandateCardWidgetProps {
  actionDescription?: string;
  estimatedCostUsd?: number;
  targetBackend?: string;
  // J4: lab tenant the mandate authorizes against. Absent means the
  // authorizeSpend tenant gate refuses (no default attribution) — the
  // refusal renders verbatim in the sign error, never a silent default.
  tenantId?: string;
  // The exact session + program the mandate authorizes. Without both,
  // authorizeSpend refuses (no unbound signatures).
  sessionId?: string;
  programQasm?: string;
  onAuthorize?: (mandateJson: string) => void;
  onDowngrade?: () => void;
  onAbort?: () => void;
}

export function CryptographicMandateCardWidget({
  actionDescription,
  estimatedCostUsd,
  targetBackend,
  tenantId,
  sessionId,
  programQasm,
  onAuthorize,
  onDowngrade,
  onAbort,
}: CryptographicMandateCardWidgetProps) {
  const [isSigning, setIsSigning] = useState(false);
  const [signedToken, setSignedToken] = useState<string | null>(null);
  const [verifiedLocal, setVerifiedLocal] = useState<boolean | null>(null);
  const [signError, setSignError] = useState<string | null>(null);
  // No demo defaults: authorization chrome with a fictional dispatch would
  // sign spend for work that was never proposed. Without props the card
  // states the gap and signing stays disabled.
  const hasProposal = actionDescription != null && estimatedCostUsd != null && targetBackend != null;
  const costFinite = typeof estimatedCostUsd === 'number' && Number.isFinite(estimatedCostUsd);

  const handleSign = async () => {
    if (!hasProposal) return;
    // Guarded above: all three are defined here. Locals keep TS honest
    // without reintroducing demo fallbacks.
    const cost = estimatedCostUsd as number;
    setIsSigning(true);
    setSignError(null);
    setVerifiedLocal(null);
    try {
      // Canonical job binding + server verification: the mandate covers
      // (tenant, session, exact program, budget) and /api/mandates/verify
      // must accept it before the card reports authorization.
      const res = await authorizeSpend({
        sessionId,
        tenantId,
        estimatedUsd: cost,
        programQasm,
      });
      if (!res.ok) {
        throw new Error(res.reason);
      }
      const mandateJson = JSON.stringify(res.mandate);
      setVerifiedLocal(true);
      setSignedToken(mandateJson);
      onAuthorize?.(mandateJson);
    } catch (err) {
      setSignError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsSigning(false);
    }
  };

  return (
    <div data-provenance={hasProposal ? 'proposed' : undefined} className="rounded-2xl border border-[var(--color-danger-edge)] bg-[var(--color-danger-wash)] p-5 text-[var(--color-text)] space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-[var(--color-danger-edge)] pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-[var(--color-soft-rose-wash)] border border-[var(--color-soft-rose-edge)] text-[var(--color-danger-ink)]">
            <KeyRound className="w-5 h-5" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-semibold text-[var(--color-danger-ink)]">AP2 High-Friction Spend Mandate Card</h3>
              {hasProposal && <ProvenanceBadge kind="proposed" detail="Spend estimate and target are proposal fields." />}
            </div>
            <p className="text-xs text-[var(--color-muted)]">Autonomous Action Requiring Cryptographic Human Authorization</p>
          </div>
        </div>
        <span className="px-2.5 py-1 rounded-full text-xs font-mono bg-[var(--color-soft-rose-wash)] border border-[var(--color-soft-rose-edge)] text-[var(--color-soft-rose-ink)]">
          Autonomy Level 2 Gate
        </span>
      </div>

      {/* Target & Cost Details */}
      {!hasProposal ? (
        <div className="p-4 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)] text-xs text-[var(--color-muted)]">
          No action proposed — nothing to authorize. This card signs only explicit proposals.
        </div>
      ) : (
      <div className="p-4 rounded-xl bg-[var(--color-card)] border border-[var(--color-danger-edge)] space-y-3 text-xs">
        <div>
          <span className="text-[var(--color-muted)] block text-[12px]">Action Request</span>
          <p className="font-semibold text-[var(--color-text)] text-sm mt-0.5">{actionDescription}</p>
        </div>
        <div className="grid grid-cols-2 gap-3 pt-2 border-t border-[var(--color-border)]">
          <div>
            <span className="text-[var(--color-muted)] block text-[12px]">Target QPU Backend</span>
            <span className="font-mono text-[var(--color-info-ink)]">{targetBackend}</span>
          </div>
          <div>
            <span className="text-[var(--color-muted)] block text-[12px]">Estimated QPU Spend</span>
            {costFinite ? (
              <span className="font-mono text-[var(--color-warning-ink)] text-sm font-bold">${(estimatedCostUsd as number).toFixed(2)} USD</span>
            ) : (
              <span className="font-mono text-[var(--color-warning-ink)] text-sm font-bold">UNPRICED — authorization disabled</span>
            )}
          </div>
        </div>
      </div>
      )}

      {/* Signature Status or Actions */}
      {signedToken ? (
        <div className="p-3 rounded-xl bg-[var(--color-success-wash)] border border-[var(--color-success-edge)] flex items-center space-x-2 text-xs text-[var(--color-success-ink)]">
          <CheckCircle2 className="w-4 h-4 text-[var(--color-success-ink)] flex-shrink-0" />
          <div className="truncate">
            <span className="font-bold">Authorized & Server-Verified{verifiedLocal === false ? ' (UNVERIFIED)' : ''}: </span>
            <span className="font-mono text-[12px] text-[var(--color-success-ink)]">{signedToken}</span>
            <div className="text-[12px] font-code text-[var(--color-muted)]">canonical session+program mandate — server-verified before dispatch</div>
          </div>
        </div>
      ) : !hasProposal ? (
        <p className="text-[12px] font-code text-[var(--color-faint)]">Authorize disabled — no proposal bound.</p>
      ) : (
        <div className="flex flex-wrap items-center gap-2 pt-1">
          <button
            onClick={handleSign}
            disabled={isSigning || !costFinite}
            title={!costFinite ? 'Disabled: unpriced — authorization disabled' : undefined}
            className="flex-1 py-2 px-4 rounded-xl bg-gradient-to-r from-rose-600 to-amber-600 hover:from-rose-500 hover:to-amber-500 text-[var(--color-on-accent)] font-medium text-xs shadow-lg transition flex items-center justify-center space-x-2 disabled:opacity-50"
          >
            <KeyRound className="w-3.5 h-3.5" />
            <span>{isSigning ? 'Signing WebCrypto...' : 'Authorize via Ed25519 Card'}</span>
          </button>
          <button
            onClick={onDowngrade}
            className="py-2 px-3 rounded-xl bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[var(--color-label-secondary)] text-xs transition flex items-center space-x-1.5"
          >
            <ArrowDownCircle className="w-3.5 h-3.5 text-[var(--color-info-ink)]" />
            <span>Downgrade to simulator</span>
          </button>
          <button
            onClick={onAbort}
            className="py-2 px-3 rounded-xl bg-[var(--color-fill)] hover:bg-[var(--color-danger-wash)] border border-[var(--color-border)] text-[var(--color-danger-ink)] text-xs transition flex items-center space-x-1.5"
          >
            <XCircle className="w-3.5 h-3.5 text-[var(--color-danger-ink)]" />
            <span>Abort</span>
          </button>
        </div>
      )}
      {signError && (
        <div className="p-2.5 rounded-xl bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-[12px]">
          Sign failed — nothing authorized, nothing dispatched ({signError.slice(0, 200)}).
        </div>
      )}
    </div>
  );
}
