'use client';

import React, { useState } from 'react';
import { KeyRound, Radio } from 'lucide-react';
import {
  fetchJob,
  jobStatusLabel,
  launchQAOAHardware,
  launchPESHardware,
  launchSpinChainHardware,
  launchVQEHardware,
  prepareQAOAHardware,
  preparePESHardware,
  prepareSpinChainHardware,
  prepareVQEHardware,
  type HardwarePrepareResult,
} from '@/lib/api';
import { createSignedSpendMandateWithCustody } from '@/lib/mandates';
import { verifySpendMandate } from '@/lib/api';
import { publishInvalidation } from '@/domain/invalidation';
import type { Job } from '@/types';

/** Operator-facing governed launch for a measured spin-chain source job.
 * Preparation returns the exact parent binding; the browser key signs only
 * that binding, and the Go rail remains the authority on mandate admission,
 * fidelity, spend, and settlement. */
export function HardwareLaunchPanel({
  sessionId,
  tenantId,
  sourceJob,
}: {
  sessionId: string | null;
  tenantId?: string | null;
  sourceJob: Job | null;
}) {
  const [backend, setBackend] = useState('ibm_fez');
  const [jurisdiction, setJurisdiction] = useState('ZM');
  const [budget, setBudget] = useState('1.00');
  const [shots, setShots] = useState('1024');
  const [loop, setLoop] = useState(false);
  const [mitigation, setMitigation] = useState(false);
  const [prepared, setPrepared] = useState<HardwarePrepareResult | null>(null);
  const [launched, setLaunched] = useState<Job | null>(null);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<string | null>(null);

  if (!sourceJob || jobStatusLabel(sourceJob.status) !== 'COMPLETED') return null;
  const result = sourceJob.result_json && typeof sourceJob.result_json === 'object'
    ? sourceJob.result_json as Record<string, unknown>
    : {};
  const sourceKind = sourceJob.job_type;
  if (!['spin_chain', 'vqe_run', 'qaoa_run', 'vqe_pes'].includes(sourceKind)) return null;
  if (sourceKind === 'spin_chain' && result.state_mode === 'exact') {
    return (
      <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 text-[12px] text-[var(--color-muted)]">
        Exact-state spin sources are simulator-only: their QASM contains reset gates that QASM2 cannot carry to hardware. Use a VQE-mode source for governed QPU launch.
      </div>
    );
  }

  const maxBudget = Number(budget);
  const canPrepare = Boolean(sessionId && backend.trim() && Number.isFinite(maxBudget) && maxBudget > 0 && Number(shots) >= 128);
  const canSign = Boolean(prepared && tenantId && sessionId && canPrepare);

  const prepare = async () => {
    if (!sessionId || !canPrepare) return;
    setBusy(true); setNote(null); setLaunched(null);
    try {
      const prep = sourceKind === 'vqe_run'
        ? prepareVQEHardware(sessionId, sourceJob.id, backend.trim(), Number(shots), loop, mitigation)
        : sourceKind === 'qaoa_run'
          ? prepareQAOAHardware(sessionId, sourceJob.id, backend.trim(), Number(shots), loop)
          : sourceKind === 'vqe_pes'
            ? preparePESHardware(sessionId, sourceJob.id, backend.trim(), Number(shots), mitigation)
            : prepareSpinChainHardware(sessionId, sourceJob.id, backend.trim(), Number(shots));
      setPrepared(await prep);
    } catch (err) {
      setNote(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const signAndLaunch = async () => {
    if (!prepared || !sessionId || !tenantId || !canPrepare) return;
    setBusy(true); setNote(null);
    try {
      const mandate = await createSignedSpendMandateWithCustody({
        tenantId, jobHash: prepared.binding, maxBudgetUsd: maxBudget, ttlSeconds: 900,
      });
      const verified = await verifySpendMandate(mandate);
      if (!verified.valid || !verified.receipt) {
        throw new Error(verified.error || 'mandate verification returned no ledger receipt');
      }
      const launch = sourceKind === 'vqe_run'
        ? launchVQEHardware(sessionId, sourceJob.id, verified.receipt, backend.trim(), Number(shots), maxBudget, `ui-vqe-hw-${Date.now()}`, jurisdiction.trim() || 'ZM', loop, mitigation)
        : sourceKind === 'qaoa_run'
          ? launchQAOAHardware(sessionId, sourceJob.id, verified.receipt, backend.trim(), Number(shots), maxBudget, `ui-qaoa-hw-${Date.now()}`, jurisdiction.trim() || 'ZM', loop)
          : sourceKind === 'vqe_pes'
            ? launchPESHardware(sessionId, sourceJob.id, verified.receipt, backend.trim(), Number(shots), maxBudget, `ui-pes-hw-${Date.now()}`, jurisdiction.trim() || 'ZM', mitigation)
            : launchSpinChainHardware(sessionId, sourceJob.id, verified.receipt, backend.trim(), Number(shots), maxBudget, `ui-spin-hw-${Date.now()}`, jurisdiction.trim() || 'ZM');
      const job = await launch;
      if (!job) throw new Error('launch returned no job');
      let current: Job | null = job;
      setLaunched(current);
      for (let i = 0; i < 120; i++) {
        await new Promise((resolve) => setTimeout(resolve, 1000));
        current = await fetchJob(current.id, sessionId);
        if (!current) break;
        setLaunched(current);
        const st = jobStatusLabel(current.status).toUpperCase();
        if (st !== 'QUEUED' && st !== 'RUNNING') break;
      }
      publishInvalidation(new Set(['campaign', 'jobs', 'obligations', 'loops', 'cost']));
    } catch (err) {
      setNote(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-2xl border border-amber-500/30 bg-amber-950/10 p-4 space-y-3" aria-label="Governed hardware launch">
      <div className="flex items-center gap-2">
        <Radio className="w-4 h-4 text-amber-300" />
        <span className="text-xs font-semibold text-white">Governed QPU launch</span>
      </div>
      <p className="text-[12px] text-[var(--color-muted)]">Prepares the exact {sourceKind === 'vqe_run' ? 'VQE' : sourceKind === 'qaoa_run' ? 'QAOA' : sourceKind === 'vqe_pes' ? 'PES' : 'spin-chain'} parent binding, signs it with the browser WebCrypto key, then submits through mandate, fidelity, reservation, and settlement gates. Nothing here bypasses the Go rail.</p>
      <div className="grid grid-cols-2 gap-2 text-[12px]">
        <label className="space-y-1">Backend
          <input aria-label="Hardware backend" value={backend} onChange={(e) => setBackend(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
        </label>
        <label className="space-y-1">Max budget USD
          <input aria-label="Hardware max budget" value={budget} onChange={(e) => setBudget(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
        </label>
        <label className="space-y-1">Shots
          <input aria-label="Hardware shots" value={shots} onChange={(e) => setShots(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
        </label>
        <label className="space-y-1">Jurisdiction
          <input aria-label="Hardware jurisdiction" value={jurisdiction} onChange={(e) => setJurisdiction(e.target.value)} className="w-full rounded bg-black/60 border border-white/10 px-2 py-1" />
        </label>
        {sourceKind !== 'spin_chain' && sourceKind !== 'vqe_pes' && (
          <label className="col-span-2 flex items-center gap-2 text-[12px] text-[var(--color-muted)]">
            <input type="checkbox" aria-label="Hardware loop" checked={loop} onChange={(e) => { setLoop(e.target.checked); if (e.target.checked) setMitigation(false); }} />
            Hardware-in-the-loop optimizer (bounded)
          </label>
        )}
        {sourceKind !== 'spin_chain' && sourceKind !== 'qaoa_run' && (
          <label className="col-span-2 flex items-center gap-2 text-[12px] text-[var(--color-muted)]">
            <input type="checkbox" aria-label="Hardware ZNE mitigation" checked={mitigation} disabled={loop} onChange={(e) => setMitigation(e.target.checked)} />
            Zero-noise extrapolation (ZNE, scales 1/3/5; result stays measured=false)
          </label>
        )}
      </div>
      <p className="text-[12px] text-[var(--color-muted)]">Vendor price is not exposed before dispatch. Set the operator max budget explicitly; the parent rail enforces it against the signed mandate, reserving it before any QPU batch.</p>
      {!tenantId && <p role="alert" className="text-[12px] text-amber-300">No lab tenant on this session: signing is disabled.</p>}
      {note && <p role="alert" className="text-[12px] text-rose-300">{note}</p>}
      <div className="flex flex-wrap gap-2">
        <button type="button" onClick={() => void prepare()} disabled={busy || !canPrepare} className="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/[0.05] border border-white/10 text-[var(--color-pulse-cyan)] text-[12px] disabled:opacity-40">
          <Radio className="w-3.5 h-3.5" /> Prepare binding
        </button>
        <button type="button" onClick={() => void signAndLaunch()} disabled={busy || !canSign} className="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-amber-500/10 border border-amber-400/30 text-amber-200 text-[12px] disabled:opacity-40">
          <KeyRound className="w-3.5 h-3.5" /> Sign &amp; launch
        </button>
      </div>
      {prepared && (
        <div className="rounded-xl bg-white/[0.03] border border-white/[0.06] p-2 text-[12px] font-code space-y-1 break-all">
          <div>binding {prepared.binding.slice(0, 22)}…</div>
          <div>backend {String(prepared.spec.requested_backend || backend)} · shots {String(prepared.spec.shots || shots)} · max ${String(prepared.spec.max_cost_usd || budget)}</div>
          {prepared.pricing && (
            <div>
              {prepared.pricing.available === true
                ? `pricing ${String(prepared.pricing.plan || 'reviewed')} · ${String(prepared.pricing.pricing_type || 'rate')} · estimated $${String(prepared.pricing.estimated_usd ?? 'unknown')}`
                : `pricing unavailable · ${String(prepared.pricing.reason || 'no reviewed QPU rate row')}`}
            </div>
          )}
        </div>
      )}
      {launched && (
        <div className="rounded-xl bg-white/[0.03] border border-white/[0.06] p-2 text-[12px] space-y-0.5">
          <div>hardware job {launched.id.slice(0, 8)} · {launched.status}</div>
          {jobStatusLabel(launched.status) === 'COMPLETED' && typeof (launched.result_json as Record<string, unknown>)?.hardware_job_id === 'string' && (
            <div>vendor job {String((launched.result_json as Record<string, unknown>).hardware_job_id)}</div>
          )}
          {jobStatusLabel(launched.status) === 'FAILED' && <div className="text-rose-300">failed — see the job record</div>}
        </div>
      )}
    </div>
  );
}
