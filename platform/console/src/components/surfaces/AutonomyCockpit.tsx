'use client';

import React, { useState, useEffect } from 'react';
import { KeyRound, ShieldCheck, AlertOctagon, Sparkles, ChevronDown, ChevronUp } from 'lucide-react';
import { CampaignState, Session } from '@/types';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { launchJobId, newWriteRequestId, reconcileOperationFull, UNCONFIRMED_LAUNCH, writeSettlementLabel } from '@/domain/write-settlement';
import { emergencyAbort, launchProposal, mintProposal, fetchLLMSettings, fetchBackendStatus, type BackendStatus } from '@/lib/api';
import { AirGapSwitchRow, useAirGapControl } from '@/components/common/AirGapControl';
import { ApprovalPacketCard } from '@/components/common/ApprovalPacketCard';
import ConfirmDialog from '@/components/common/ConfirmDialog';
import { MetasearchPanel } from '@/components/common/MetasearchPanel';
import { ObligationsPanel } from '@/components/common/ObligationsPanel';
import { AutoresearchPanel } from '@/components/common/AutoresearchPanel';
import { WorkflowPanel } from '@/components/common/WorkflowPanel';
import { buildApprovalPacketFromCampaign } from '@/lib/approvalPacket';
import { authorizeSpend } from '@/lib/authorizeSpend';
import { subscribeInvalidation } from '@/domain/invalidation';
import { fmtFixed } from '@/lib/guards';

interface AutonomyCockpitProps {
  campaign?: CampaignState | null;
  session?: Session | null;
}

export function AutonomyCockpit({ campaign, session }: AutonomyCockpitProps) {
  // No silent money defaults: an absent estimate disables authorization
  // (same costKnown gate as ActionGate — never an invented ??0.12 cap).
  const rawEstimated = campaign?.quantum_factors?.estimated_spend_usd;
  const costKnown = typeof rawEstimated === 'number' && Number.isFinite(rawEstimated);
  const estimatedCost = costKnown ? (rawEstimated as number) : 0;
  const recommendedBackend = campaign?.quantum_factors?.recommended_backend || 'Local simulator';
  const targetName = campaign?.target_system || 'Target Molecular System';
  // Same session gate as ActionGate: no bound session, no authorization.
  const sessionBound = Boolean((session?.id ?? '').trim());

  const [budgetUSD, setBudgetUSD] = useState(session?.spend_limit_usd ?? 25.0);
  // Server spend only: refreshed from the session record, never incremented
  // locally. A local prev+estimated increment would invent ledger spend.
  const [currentSpentUSD, setCurrentSpentUSD] = useState(session?.current_spend_usd ?? 0);
  // Live Sentinel predicates (Phase-2 credibility — never static chrome):
  // airGap starts UNKNOWN (null) and is set only by live settings; backend
  // starts unloaded (null) and is set only by the readiness probe. The
  // badge derives from these — no predicate, no assurance.
  const [backend, setBackend] = useState<BackendStatus | null>(null);
  const [lastReceipt, setLastReceipt] = useState<{ sigPreview: string; verifiedAt: string; tenant: string } | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [showCryptoDetails, setShowCryptoDetails] = useState(false);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const {
    airGap,
    setAirGap,
    settlementLabel: airGapSettlementLabel,
    toggle: handleToggleAirGap,
  } = useAirGapControl({
    onStatusMessage: (msg) => {
      setActionMessage(msg);
      if (msg) setTimeout(() => setActionMessage(null), 4000);
    },
  });

  // No stale budget: resync planning view whenever the session changes.
  // Reset during render (key-change pattern, not in the effect below):
  // the budget fields must track the newly selected session, never the
  // previous one's figures.
  const sessionId = session?.id ?? '';
  const [prevSessionId, setPrevSessionId] = useState(sessionId);
  if (prevSessionId !== sessionId) {
    setPrevSessionId(sessionId);
    setBudgetUSD(session?.spend_limit_usd ?? 25.0);
    setCurrentSpentUSD(session?.current_spend_usd ?? 0);
  }

  // Initialize the switch from live settings — never assume engaged.
  // Unreachable settings leave airGap null (unknown): the badge demotes
  // to neutral copy and the toggle disables until the state is known.
  useEffect(() => {
    let mounted = true;
    fetchLLMSettings()
      .then((d) => {
        if (mounted && d && typeof (d as { air_gapped?: boolean }).air_gapped === 'boolean') {
          setAirGap((d as { air_gapped: boolean }).air_gapped);
        }
      })
      .catch(() => {});
    const unsub = subscribeInvalidation(['llm'], () => {
      void fetchLLMSettings()
        .then((d) => {
          if (mounted && d && typeof (d as { air_gapped?: boolean }).air_gapped === 'boolean') {
            setAirGap((d as { air_gapped: boolean }).air_gapped);
          }
        })
        .catch(() => {});
    });
    return () => {
      mounted = false;
      unsub();
    };
  }, [setAirGap]);

  // Backend readiness probe (same /api/health + /api/ready as the status
  // bar): the second Sentinel predicate. Transport failure leaves backend
  // null — badge demotes, never claims.
  useEffect(() => {
    let mounted = true;
    fetchBackendStatus()
      .then((s) => {
        if (mounted) setBackend(s);
      })
      .catch(() => {});
    return () => {
      mounted = false;
    };
  }, []);

  const handleApprovePendingRun = async () => {
    setIsSubmitting(true);
    try {
      // Session-bound, priced-packet-only authorization via the shared
      // helper (same gates as ActionGate: session + priced + program).
      // Verify-only: a valid mandate is reported as verified, never as
      // dispatched — this surface mints no ledger spend locally.
      const res = await authorizeSpend({ session, campaign });
      if (!res.ok) {
        setActionMessage(`${res.reason} Halted fail-closed — nothing dispatched, nothing spent.`);
        setTimeout(() => setActionMessage(null), 6000);
        return;
      }
      const sigPreview = res.mandate.signature_hex.slice(0, 16);
      setLastReceipt({ sigPreview, verifiedAt: new Date().toISOString(), tenant: res.tenantId });
      if (!session?.id || !campaign?.quantum_factors?.qasm) {
        setActionMessage('Mandate verified, but no bound session/program exists — nothing was dispatched.');
        setTimeout(() => setActionMessage(null), 6000);
        return;
      }
      // A verified mandate is a capability, not a run. Hand the exact
      // program + signed mandate to the Go launch rail and report the real
      // job id; otherwise the operator would see an approval that never ran.
      const requestId = newWriteRequestId();
      const writeId = launchWrite.begin('launch proposal');
      // B1: the server prices and owns the plan; this surface names it.
      const minted = await mintProposal(session.id, 'campaign');
      if (!minted.ok) {
        launchWrite.refused(writeId, minted.error);
        setActionMessage(`No launchable plan: ${minted.error}`);
        setTimeout(() => setActionMessage(null), 8000);
        return;
      }
      const backend = minted.proposal.body.backend || recommendedBackend;
      const launched = await launchProposal(
        session.id,
        { proposalId: minted.proposal.id, digest: minted.proposal.digest },
        { mandateRef: JSON.stringify(res.mandate), requestId },
      );
      if ('outcome' in launched) {
        const settled = await reconcileOperationFull(requestId, session.id);
        const jobId = settled.phase === 'applied' ? launchJobId(settled.receipt) : null;
        if (jobId) {
          launchWrite.applied(writeId, `job ${jobId.slice(0, 8)} dispatched`);
          setActionMessage(`Mandate verified (Sig: ${sigPreview}...). Dispatched job ${jobId.slice(0, 8)} to ${backend}.`);
        } else if (settled.phase === 'refused') {
          launchWrite.refused(writeId, settled.detail);
          setActionMessage(`Mandate verified, but dispatch was refused: ${settled.detail}`);
        } else {
          launchWrite.unknown(writeId, UNCONFIRMED_LAUNCH);
          setActionMessage(UNCONFIRMED_LAUNCH);
        }
      } else if ('error' in launched) {
        launchWrite.refused(writeId, launched.error);
        setActionMessage(`Mandate verified, but dispatch was refused: ${launched.error}`);
      } else {
        launchWrite.applied(writeId, `job ${String(launched.job.id).slice(0, 8)} dispatched`);
        setActionMessage(`Mandate verified (Sig: ${sigPreview}...). Dispatched job ${String(launched.job.id).slice(0, 8)} to ${backend}.`);
      }
      setTimeout(() => setActionMessage(null), 6000);
    } catch (err) {
      console.error('Failed to authorize run:', err);
      setActionMessage('Failed to sign mandate envelope. Action halted fail-closed.');
      setTimeout(() => setActionMessage(null), 4000);
    } finally {
      setIsSubmitting(false);
    }
  };

  // Real local dispatch: builds a simulator-tier proposal from the live
  // campaign circuit and launches it. Disabled without session + QASM —
  // never a banner pretending work happened.
  const handleSimulateOffline = async () => {
    const qf = campaign?.quantum_factors;
    if (!session?.id || !qf?.qasm) return;
    setIsSubmitting(true);
    const writeId = launchWrite.begin('offline launch');
    const requestId = newWriteRequestId();
    try {
      // B1: offline dispatch mints a server-owned simulator downgrade — the
      // circuit and price come from stored campaign state, never from here.
      const minted = await mintProposal(session.id, 'campaign', { downgrade: 'simulator' });
      if (!minted.ok) {
        launchWrite.refused(writeId, minted.error);
        setActionMessage(`Local dispatch refused: ${minted.error}`);
        setTimeout(() => setActionMessage(null), 8000);
        return;
      }
      const res = await launchProposal(
        session.id,
        { proposalId: minted.proposal.id, digest: minted.proposal.digest },
        { requestId },
      );
      if (res && 'job' in res && res.job) {
        launchWrite.applied(writeId, `job ${String(res.job.id).slice(0, 8)} dispatched offline`);
        setActionMessage(`Dispatched offline: job ${String(res.job.id).slice(0, 8)} on local simulator.`);
      } else if (res && 'error' in res) {
        launchWrite.refused(writeId, res.error);
        setActionMessage(`Local dispatch refused: ${res.error}`);
      } else {
        const settled = await reconcileOperationFull(requestId, session.id);
        const jobId = settled.phase === 'applied' ? launchJobId(settled.receipt) : null;
        if (jobId) {
          launchWrite.applied(writeId, `job ${jobId.slice(0, 8)} dispatched offline`);
          setActionMessage(`Dispatched offline: job ${jobId.slice(0, 8)} on local simulator.`);
        } else if (settled.phase === 'refused') {
          launchWrite.refused(writeId, settled.detail);
          setActionMessage(`Local dispatch refused: ${settled.detail}`);
        } else {
          launchWrite.unknown(writeId, UNCONFIRMED_LAUNCH);
          setActionMessage(UNCONFIRMED_LAUNCH);
        }
      }
    } catch {
      launchWrite.unknown(writeId, UNCONFIRMED_LAUNCH);
      setActionMessage(UNCONFIRMED_LAUNCH);
    } finally {
      setIsSubmitting(false);
      setTimeout(() => setActionMessage(null), 5000);
    }
  };

  // Air-gap toggle is delegated to useAirGapControl + <AirGapSwitchRow />
  // for shared optimistic persist + revert-on-failure behavior.

  const handleEmergencyStop = async () => {    const sid = session?.id || campaign?.session_id || '';
    if (!sid) {
      setActionMessage('No active session: open a campaign before triggering an abort.');
      setTimeout(() => setActionMessage(null), 4000);
      return;
    }
    setAbortOpen(true);
  };

  const [abortOpen, setAbortOpen] = useState(false);
  const [abortBusy, setAbortBusy] = useState(false);
  const abortWrite = useWriteSettlement();
  const launchWrite = useWriteSettlement();
  const confirmAbort = async () => {
    const sid = session?.id || campaign?.session_id || '';
    if (!sid) {
      setAbortOpen(false);
      return;
    }
    setAbortBusy(true);
    const requestId = abortWrite.begin('emergency abort');
    try {
      const res = await emergencyAbort(sid, 'emergency pause from guardrails');
      if (res.ok) {
        abortWrite.applied(requestId, `${res.aborted_count} job(s) settled failed`);
        setActionMessage(`Ring 0 Intercept complete: ${res.aborted_count} job(s) settled failed. Receipt: ${res.abort_receipt}`);
        setAbortOpen(false);
      } else {
        abortWrite[res.unknown ? 'unknown' : 'refused'](requestId, res.error);
        // Keep the dialog open on refusal so the operator can retry or cancel.
        setActionMessage(`Abort ${res.unknown ? 'unconfirmed' : 'refused'} — nothing was settled. Retry or use ⌘. from the arena.`);
      }
    } finally {
      setAbortBusy(false);
    }
    setTimeout(() => setActionMessage(null), 6000);
  };

  // Sentinel badge predicate (Phase-2 credibility): ACTIVE (emerald) iff
  // the backend is reachable AND ready AND the sovereign air-gap is
  // engaged per live settings. Reachable predicates that fail either
  // condition render DEGRADED (amber) with the live reason; unreachable
  // predicates render NEUTRAL (slate) controls copy — never assurance.
  const sentinelActive =
    airGap === true && backend !== null && backend.reachable && backend.ready;
  const sentinelPredicatesLive =
    airGap !== null && backend !== null && backend.reachable;
  const sentinelReason: string | null = !sentinelPredicatesLive
    ? null
    : backend !== null && !backend.ready
      ? backend.message || 'backend not ready'
      : 'sovereign air-gap released';
  const sessionTenantLabel = (session?.tenant_id ?? '').trim() || 'unbound';

  return (
    <div className="relative min-h-[640px] rounded-sheet overflow-hidden bg-page border border-border shadow-xl flex flex-col justify-between p-8 select-none">
      {/* Top Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-border pb-5">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-card bg-tile border border-border flex items-center justify-center">
            <KeyRound className="w-5 h-5 text-warning-ink" />
          </div>
          <div>
            <h2 className="text-body-large font-semibold text-text tracking-tight">Autonomous Guardrails &amp; Sentinel</h2>
            <p className="text-label text-muted mt-0.5">
              Secure spending limits, Sentinel action approval, and sovereign air-gap isolation
            </p>
          </div>
        </div>

        {/* Sentinel Shield Badge — derived from live predicates only:
            backend reachable+ready AND air-gap engaged (see above).
            Degraded names the live reason; unknown predicates demote to
            neutral controls copy. No static assurance chrome remains. */}
        <div
          data-testid="sentinel-badge"
          title={
            sentinelActive
              ? 'Live: backend ready and sovereign air-gap engaged'
              : sentinelPredicatesLive
                ? `Live: ${sentinelReason}`
                : 'Predicates unreachable — status unknown, no assurance claimed'
          }
          className={`flex items-center space-x-2 px-3.5 py-1.5 rounded-capsule ${
            sentinelActive
              ? 'bg-success-wash border border-success-edge'
              : sentinelPredicatesLive
                ? 'bg-warning-wash border border-warning-edge'
                : 'bg-tile border border-border'
          }`}
        >
          <ShieldCheck
            className={`w-4 h-4 ${sentinelActive ? 'text-success-ink' : sentinelPredicatesLive ? 'text-warning-ink' : 'text-muted'}`}
          />
          <span
            className={`text-label font-mono font-medium ${sentinelActive ? 'text-success-ink' : sentinelPredicatesLive ? 'text-warning-ink' : 'text-muted'}`}
          >
            {sentinelActive
              ? 'Sentinel Protection Active'
              : sentinelPredicatesLive
                ? `Sentinel degraded — ${sentinelReason}`
                : 'Sentinel controls enabled'}
          </span>
        </div>
      </div>

      {/* Main Guardrail Stage */}
      <div className="flex-1 flex flex-col items-center justify-center my-6 max-w-xl mx-auto w-full space-y-6">
        {/* Action Message Banner */}
        {actionMessage && (
          <div className="w-full p-3.5 rounded-card bg-info-wash border border-info-edge text-info-ink text-label font-medium text-center animate-pulse">
            {actionMessage}
          </div>
        )}
        {airGapSettlementLabel && (
          <p role="status" className="text-[12px] font-mono text-muted">
            {airGapSettlementLabel}
          </p>
        )}
        {writeSettlementLabel(abortWrite.write) && (
          <p role="status" className="text-[12px] font-mono text-muted">
            {writeSettlementLabel(abortWrite.write)}
          </p>
        )}
        {writeSettlementLabel(launchWrite.write) && (
          <p role="status" className="text-[12px] font-mono text-muted">
            {writeSettlementLabel(launchWrite.write)}
          </p>
        )}

        {/* Meta Muse-Inspired Proactive Approval Card */}
        <div className="w-full bg-surface border border-border rounded-sheet p-6 shadow-md space-y-4">
          <div className="flex items-center justify-between">
            <span className="text-label font-semibold text-text flex items-center space-x-2">
              <Sparkles className="w-4 h-4 text-info-ink" />
              <span>Sentinel Approval Request</span>
            </span>
            <span className="text-[12px] font-mono text-info-ink bg-info-wash px-2.5 py-0.5 rounded-capsule border border-info-edge">
              {costKnown ? `Est. Cost: $${fmtFixed(rawEstimated, 2)}` : 'UNPRICED — authorization disabled'}
            </span>
          </div>

          <p className="text-body-strong text-label-secondary leading-relaxed font-sans">
            Ansatz Scout has prepared an optimized variational trial for {targetName}. Would you like to run 1,024 shots on the {recommendedBackend}?
          </p>
          {!sessionBound && (
            <p className="text-[12px] font-code text-warning-ink">No session bound — authorization disabled until a session is attached.</p>
          )}
          {!costKnown && (
            <p className="text-[12px] font-code text-warning-ink">No priced estimate on this campaign — authorization disabled until priced.</p>
          )}

          <ApprovalPacketCard
            packet={buildApprovalPacketFromCampaign(
              costKnown ? (campaign ?? null) : null,
              session?.id || '',
              estimatedCost,
              session?.spend_limit_usd ?? budgetUSD,
              session?.current_spend_usd ?? currentSpentUSD
            )}
            isSubmitting={isSubmitting}
            onApproveOnce={handleApprovePendingRun}
            onApproveUpToMax={handleApprovePendingRun}
            onDeny={(reason) => {
              setActionMessage(`Run denied (${reason}). Halted fail-closed — nothing dispatched, nothing spent.`);
              setTimeout(() => setActionMessage(null), 5000);
            }}
          />
          <div className="flex items-center space-x-3 pt-2">
            {/* Money-adjacent: local simulation spends exactly $0.00 (the
                spend_estimate minted in handleSimulateOffline) — 2dp, no
                invented figure. Approve buttons carrying the priced estimate
                live in ApprovalPacketCard ("Approve up to $X.XX"). */}
            <button
              onClick={handleSimulateOffline}
              disabled={isSubmitting || !session?.id || !campaign?.quantum_factors?.qasm}
              title={!campaign?.quantum_factors?.qasm ? 'Needs a campaign circuit first' : 'Dispatch to local simulator ($0.00 spend)'}
              className="flex-1 py-2.5 px-4 bg-tile hover:bg-fill-hover text-text border border-border text-label font-medium rounded-row transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] disabled:opacity-40"
            >
              Simulate Offline — $0.00
            </button>
          </div>
        </div>

        {/* Budget Planning View (local sketch — enforcement lives in the session cap) */}
        <div className="w-full bg-surface border border-border rounded-sheet p-6 space-y-4">
          <div className="flex justify-between items-center text-label">
            <span className="font-semibold text-text">Planning Ceiling <span className="text-muted font-normal">(local view)</span></span>
            <span className="font-mono text-success-ink font-bold">${fmtFixed(budgetUSD, 2)} Maximum</span>
          </div>

          <input 
            type="range"
            min="5"
            max="100"
            step="5"
            value={budgetUSD}
            onChange={(e) => setBudgetUSD(parseFloat(e.target.value))}
            className="w-full h-2 bg-tile rounded-control-compact appearance-none cursor-pointer accent-success focus:outline-none"
          />

          <div className="flex justify-between items-center text-[12px] text-muted font-mono pt-1">
            <span>Spent (server): ${fmtFixed(session?.current_spend_usd ?? currentSpentUSD, 2)}</span>
            <span>Available: ${fmtFixed((session?.spend_limit_usd ?? budgetUSD) - (session?.current_spend_usd ?? currentSpentUSD), 2)}</span>
          </div>
          <p className="text-[12px] font-code text-muted">Spend refreshes from the session/ledger only — this view never invents ledger spend.</p>
        </div>

        {/* Sovereign Air-Gap Toggle & Emergency Pause */}
        <AirGapSwitchRow value={airGap} onToggle={() => void handleToggleAirGap()} />

        {/* U1-d: meta-search strategy proposals — the human-approval half
            of the autonomy loop. Approve/reject/apply are bearer-gated;
            refusals render with the backend reason. */}
        <MetasearchPanel />
        <AutoresearchPanel sessionId={session?.id ?? ''} />
        <WorkflowPanel sessionId={session?.id ?? ''} />
        <ObligationsPanel />
      </div>

      {/* Bottom Footer: Emergency Pause & Progressive Disclosure */}
      <div className="border-t border-border pt-4 flex flex-wrap items-center justify-between gap-4">
        {/* Tactile Emergency Pause */}
        <button
          onClick={handleEmergencyStop}
          className="px-3.5 py-2 rounded-row bg-danger-wash hover:opacity-90 border border-danger-edge text-danger-ink text-label font-semibold flex items-center space-x-2 transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] active:scale-95"
        >
          <AlertOctagon className="w-4 h-4 text-danger-ink" />
          <span>Emergency Pause All Workloads</span>
        </button>
        <ConfirmDialog
          open={abortOpen}
          title="Emergency abort — settle running work as failed?"
          body="This settles every unsettled job in this session as FAILED. Type the session short-id to confirm."
          requireText={(session?.id || campaign?.session_id || '').slice(0, 8)}
          confirmLabel="Abort all running work"
          danger
          busy={abortBusy}
          onConfirm={() => void confirmAbort()}
          onCancel={() => { if (!abortBusy) setAbortOpen(false); }}
        />

        {/* Progressive Disclosure: Crypto Attestation */}
        <button
          onClick={() => setShowCryptoDetails(!showCryptoDetails)}
          className="text-label text-label-secondary hover:text-text font-medium transition-colors flex items-center space-x-1.5"
        >
          <span>{showCryptoDetails ? 'Hide Attestation' : 'Inspect Cryptographic AP2 Mandate Envelope'}</span>
          {showCryptoDetails ? <ChevronUp className="w-3.5 h-3.5 text-muted" /> : <ChevronDown className="w-3.5 h-3.5 text-muted" />}
        </button>
      </div>

      {/* Expanded Cryptographic Detail Sheet */}
      {showCryptoDetails && (
        <div className="mt-4 p-4 rounded-card bg-surface border border-border text-label font-mono text-label-secondary space-y-2.5">
          <div className="flex justify-between items-center">
            <span className="text-muted font-medium">Signing Scheme:</span>
            <span className="text-text font-bold">RFC 8032 Ed25519 WebCrypto</span>
          </div>
          <div className="flex justify-between items-center">
            <span className="text-muted font-medium">Storage Invariant:</span>
            <span className="text-success-ink font-bold">SQLite WAL Tamper-Evident Hash Chain</span>
          </div>
          <div className="truncate text-label text-label-secondary pt-1 border-t border-border">
            Tenant: {sessionTenantLabel} • {lastReceipt ? `Envelope verified ${lastReceipt.verifiedAt} (sig ${lastReceipt.sigPreview}…) for tenant ${lastReceipt.tenant}` : 'No envelope verified yet — authorize a run to mint one.'}
          </div>
        </div>
      )}
    </div>
  );
}
