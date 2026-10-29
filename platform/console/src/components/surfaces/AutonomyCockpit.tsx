'use client';

import React, { useState, useEffect } from 'react';
import { KeyRound, ShieldCheck, AlertOctagon, Sparkles, ChevronDown, ChevronUp, Lock } from 'lucide-react';
import { CampaignState, Session } from '@/types';
import { emergencyAbort, launchProposal, fetchLLMSettings, saveLLMSettings, fetchBackendStatus, type BackendStatus } from '@/lib/api';
import { ApprovalPacketCard } from '@/components/common/ApprovalPacketCard';
import ConfirmDialog from '@/components/common/ConfirmDialog';
import { MetasearchPanel } from '@/components/common/MetasearchPanel';
import { buildApprovalPacketFromCampaign } from '@/lib/approvalPacket';
import { authorizeSpend } from '@/lib/authorizeSpend';
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
  const [airGap, setAirGap] = useState<boolean | null>(null);
  const [backend, setBackend] = useState<BackendStatus | null>(null);
  // Per-dispatch receipt: set only when authorizeSpend verifies a mandate
  // in this mount. The attestation sheet derives from it — no receipt, no
  // "verified" claim.
  const [lastReceipt, setLastReceipt] = useState<{ sigPreview: string; verifiedAt: string; tenant: string } | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [showCryptoDetails, setShowCryptoDetails] = useState(false);
  const [actionMessage, setActionMessage] = useState<string | null>(null);

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
    return () => {
      mounted = false;
    };
  }, []);

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
      setActionMessage(`Mandate verified (Sig: ${sigPreview}...). Verified, not dispatched — no spend recorded. Refresh the session to see ledger spend.`);
      setTimeout(() => setActionMessage(null), 5000);
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
    try {
      const res = await launchProposal(session.id, {
        target: `${campaign?.target_system || 'Target'} local simulation`,
        spend_estimate: '$0.00 (Local Simulator)',
        recommended_tier: 'Local Simulator (qiskit-aer-simulator)',
        classical_anchor: 'session anchor',
        qasm: qf.qasm,
      });
      if (res && 'job' in res && res.job) {
        setActionMessage(`Dispatched offline: job ${String(res.job.id).slice(0, 8)} on local simulator.`);
      } else if (res && 'error' in res) {
        setActionMessage(`Local dispatch refused: ${res.error}`);
      } else {
        setActionMessage('Local dispatch refused — nothing dispatched, nothing spent.');
      }
    } catch {
      setActionMessage('Local dispatch failed — nothing dispatched.');
    } finally {
      setIsSubmitting(false);
      setTimeout(() => setActionMessage(null), 5000);
    }
  };

  // Air-gap switch: Wave-2 staging decision — this surface keeps
  // IMMEDIATE persist with revert-on-failure (fetch-merge-save below) and
  // does NOT add a "staged — Save to apply" pending state. Why: the toggle
  // is a single boolean mirrored from live intelligence settings (same
  // control as Cognitive Mesh); a staged Save would add pending/dirty +
  // Save/discard plumbing for one bit with no safety gain — the current
  // path is already fail-closed (revert + banner, nothing silently kept).
  // NOTE: no DSML toggle lives on this surface; DisableDSMLFallback lives
  // in LLMSettings (out of scope), so only the Sovereign Air-Gap toggle is
  // governed here.
  const handleToggleAirGap = async () => {
    const next = !airGap;
    const prev = airGap;
    setAirGap(next);
    try {
      const current = await fetchLLMSettings();
      if (!current) throw new Error('settings unreachable');
      const res = await saveLLMSettings({ ...(current as Record<string, unknown>), air_gapped: next });
      if (res.ok) {
        setActionMessage(`Sovereign Air-Gap ${next ? 'ENGAGED' : 'released'} (persisted).`);
      } else {
        setAirGap(prev);
        setActionMessage(`Air-gap save refused — switch reverted (${res.error || 'no reason'}). Nothing changed.`);
      }
    } catch (e) {
      setAirGap(prev);
      setActionMessage(`Air-gap save failed — switch reverted, nothing changed (${e instanceof Error ? e.message : String(e)}).`);
    }
    setTimeout(() => setActionMessage(null), 4000);
  };

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
  const confirmAbort = async () => {
    const sid = session?.id || campaign?.session_id || '';
    if (!sid) {
      setAbortOpen(false);
      return;
    }
    setAbortBusy(true);
    try {
      const res = await emergencyAbort(sid, 'emergency pause from guardrails');
      if (res) {
        setActionMessage(`Ring 0 Intercept complete: ${res.aborted_count} job(s) settled failed. Receipt: ${res.abort_receipt}`);
        setAbortOpen(false);
      } else {
        // Keep the dialog open on refusal so the operator can retry or cancel.
        setActionMessage('Abort refused or failed — nothing was settled. Retry or use ⌘. from the arena.');
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
    <div className="relative min-h-[640px] rounded-3xl overflow-hidden bg-gradient-to-b from-[#090C13] via-[#0B0F19] to-[#07090E] border border-white/[0.08] shadow-2xl flex flex-col justify-between p-8 select-none">
      {/* Top Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-white/[0.08] pb-5">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-2xl bg-white/[0.05] border border-white/10 backdrop-blur-xl flex items-center justify-center shadow-inner">
            <KeyRound className="w-5 h-5 text-amber-400" />
          </div>
          <div>
            <h2 className="text-base font-semibold text-white tracking-tight">Autonomous Guardrails &amp; Sentinel</h2>
            <p className="text-xs text-slate-400 mt-0.5">
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
          className={`flex items-center space-x-2 px-3.5 py-1.5 rounded-full backdrop-blur-xl ${
            sentinelActive
              ? 'bg-black/40 border border-white/10'
              : sentinelPredicatesLive
                ? 'bg-amber-950/40 border border-amber-800/50'
                : 'bg-black/40 border border-white/10'
          }`}
        >
          <ShieldCheck
            className={`w-4 h-4 ${sentinelActive ? 'text-emerald-400' : sentinelPredicatesLive ? 'text-amber-400' : 'text-slate-400'}`}
          />
          <span
            className={`text-xs font-mono font-medium ${sentinelActive ? 'text-emerald-300' : sentinelPredicatesLive ? 'text-amber-300' : 'text-slate-400'}`}
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
          <div className="w-full p-3.5 rounded-2xl bg-cyan-950/80 border border-cyan-500/50 text-cyan-200 text-xs font-medium text-center animate-pulse">
            {actionMessage}
          </div>
        )}

        {/* Meta Muse-Inspired Proactive Approval Card */}
        <div className="w-full bg-black/60 border border-white/10 rounded-3xl p-6 backdrop-blur-2xl shadow-xl space-y-4">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-white flex items-center space-x-2">
              <Sparkles className="w-4 h-4 text-cyan-400" />
              <span>Sentinel Approval Request</span>
            </span>
            <span className="text-[11px] font-mono text-cyan-300 bg-cyan-950/80 px-2.5 py-0.5 rounded-full border border-cyan-800/40">
              {costKnown ? `Est. Cost: $${fmtFixed(rawEstimated, 2)}` : 'UNPRICED — authorization disabled'}
            </span>
          </div>

          <p className="text-sm text-slate-300 leading-relaxed font-sans">
            Ansatz Scout has prepared an optimized variational trial for {targetName}. Would you like to run 1,024 shots on the {recommendedBackend}?
          </p>
          {!sessionBound && (
            <p className="text-[11px] font-code text-amber-300">No session bound — authorization disabled until a session is attached.</p>
          )}
          {!costKnown && (
            <p className="text-[11px] font-code text-amber-300">No priced estimate on this campaign — authorization disabled until priced.</p>
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
              className="flex-1 py-2.5 px-4 bg-white/05 hover:bg-white/10 text-slate-300 text-xs font-medium rounded-xl transition-all disabled:opacity-40"
            >
              Simulate Offline — $0.00
            </button>
          </div>
        </div>

        {/* Budget Planning View (local sketch — enforcement lives in the session cap) */}
        <div className="w-full bg-black/50 border border-white/10 rounded-3xl p-6 backdrop-blur-xl space-y-4">
          <div className="flex justify-between items-center text-xs">
            <span className="font-semibold text-white">Planning Ceiling <span className="text-slate-500 font-normal">(local view)</span></span>
            <span className="font-mono text-emerald-400 font-bold">${fmtFixed(budgetUSD, 2)} Maximum</span>
          </div>

          <input 
            type="range"
            min="5"
            max="100"
            step="5"
            value={budgetUSD}
            onChange={(e) => setBudgetUSD(parseFloat(e.target.value))}
            className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-emerald-400 focus:outline-none"
          />

          <div className="flex justify-between items-center text-[11px] text-slate-400 font-mono pt-1">
            <span>Spent (server): ${fmtFixed(session?.current_spend_usd ?? currentSpentUSD, 2)}</span>
            <span>Available: ${fmtFixed((session?.spend_limit_usd ?? budgetUSD) - (session?.current_spend_usd ?? currentSpentUSD), 2)}</span>
          </div>
          <p className="text-[10px] font-code text-slate-500">Spend refreshes from the session/ledger only — this view never invents ledger spend.</p>
        </div>

        {/* Sovereign Air-Gap Toggle & Emergency Pause */}
        <div className="w-full flex items-center justify-between p-4 rounded-2xl bg-black/40 border border-white/10">
          <div className="flex items-center space-x-3">
            <div className={`w-8 h-8 rounded-xl flex items-center justify-center ${airGap === true ? 'bg-cyan-950 text-cyan-400 border border-cyan-800' : 'bg-slate-900 text-slate-400'}`}>
              <Lock className="w-4 h-4" />
            </div>
            <div>
              <div className="text-xs font-semibold text-white">Sovereign Air-Gap Mode</div>
              <div className="text-[11px] text-slate-400">
                {airGap === null ? 'State unknown — settings unreachable' : 'Strictly keep computations on local Mac hardware'}
              </div>
            </div>
          </div>

          <button
            onClick={handleToggleAirGap}
            disabled={airGap === null}
            title={airGap === null ? 'Air-gap state unknown — settings unreachable' : 'Persists to intelligence settings (same control as Cognitive Mesh)'}
            className={`w-11 h-6 rounded-full transition-colors relative p-0.5 ${airGap === true ? 'bg-cyan-500' : 'bg-slate-700'} disabled:opacity-40`}
          >
            <div className={`w-5 h-5 rounded-full bg-white shadow-md transform transition-transform ${airGap === true ? 'translate-x-5' : 'translate-x-0'}`} />
          </button>
        </div>

        {/* U1-d: meta-search strategy proposals — the human-approval half
            of the autonomy loop. Approve/reject/apply are bearer-gated;
            refusals render with the backend reason. */}
        <MetasearchPanel />
      </div>

      {/* Bottom Footer: Emergency Pause & Progressive Disclosure */}
      <div className="border-t border-white/[0.08] pt-4 flex flex-wrap items-center justify-between gap-4">
        {/* Tactile Emergency Pause */}
        <button
          onClick={handleEmergencyStop}
          className="px-3.5 py-2 rounded-xl bg-red-950/40 hover:bg-red-950/70 border border-red-800/50 text-red-300 text-xs font-semibold flex items-center space-x-2 transition-all active:scale-95"
        >
          <AlertOctagon className="w-4 h-4 text-red-400" />
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
          className="text-xs text-slate-300 hover:text-white font-medium transition-colors flex items-center space-x-1.5"
        >
          <span>{showCryptoDetails ? 'Hide Attestation' : 'Inspect Cryptographic AP2 Mandate Envelope'}</span>
          {showCryptoDetails ? <ChevronUp className="w-3.5 h-3.5 text-slate-300" /> : <ChevronDown className="w-3.5 h-3.5 text-slate-300" />}
        </button>
      </div>

      {/* Expanded Cryptographic Detail Sheet */}
      {showCryptoDetails && (
        <div className="mt-4 p-4 rounded-2xl bg-black/80 border border-white/15 text-xs font-mono text-slate-300 space-y-2.5">
          <div className="flex justify-between items-center">
            <span className="text-slate-300 font-medium">Signing Scheme:</span>
            <span className="text-slate-100 font-bold">RFC 8032 Ed25519 WebCrypto</span>
          </div>
          <div className="flex justify-between items-center">
            <span className="text-slate-300 font-medium">Storage Invariant:</span>
            <span className="text-emerald-300 font-bold">SQLite WAL Tamper-Evident Hash Chain</span>
          </div>
          <div className="truncate text-xs text-slate-300 pt-1 border-t border-white/10">
            Tenant: {sessionTenantLabel} • {lastReceipt ? `Envelope verified ${lastReceipt.verifiedAt} (sig ${lastReceipt.sigPreview}…) for tenant ${lastReceipt.tenant}` : 'No envelope verified yet — authorize a run to mint one.'}
          </div>
        </div>
      )}
    </div>
  );
}
