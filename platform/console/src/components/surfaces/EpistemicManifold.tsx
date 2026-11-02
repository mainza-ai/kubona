'use client';

import React, { useState, useEffect } from 'react';
import { Compass, CheckCircle2, ChevronDown, ChevronUp, Scale, ShieldCheck } from 'lucide-react';
import { CampaignState } from '@/types';
import { fetchEpistemics } from '@/lib/api';
import { Chart } from '@/components/data/LazyChart';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';
import type { ChartSpec } from '@/components/data/chart';

interface EpistemicWavefunction {
  claim_id: string;
  record_id?: string;
  statement: string;
  alpha_corr: number;
  beta_cont: number;
  gamma_unv: number;
  collapsed: boolean;
  status: string;
}

interface EpistemicResponse {
  active_wavefunctions?: EpistemicWavefunction[];
}

interface EpistemicManifoldProps {
  campaign?: CampaignState | null;
  sessionId?: string | null;
}

export function EpistemicManifold({ campaign, sessionId }: EpistemicManifoldProps) {
  const [showDeepMath, setShowDeepMath] = useState(false);
  const [data, setData] = useState<EpistemicResponse | null>(null);

  const effectiveSessionId = sessionId || campaign?.session_id;

  // Loading state: true until the first fetchEpistemics settles, so the
  // empty state never flashes as if it were a failed live claim.
  // Session-change resets happen during render (campaign-key guard); the
  // effect body only fetches (repo set-state-in-effect pattern).
  const [epistemicsLoading, setEpistemicsLoading] = useState(() => Boolean(effectiveSessionId));
  const [prevEpistemicSession, setPrevEpistemicSession] = useState<string | null>(null);
  const epistemicSessionKey = effectiveSessionId ?? '';
  if (epistemicSessionKey !== (prevEpistemicSession ?? '')) {
    setPrevEpistemicSession(epistemicSessionKey);
    setEpistemicsLoading(epistemicSessionKey !== '');
  }

  useEffect(() => {
    if (!effectiveSessionId) return;
    fetchEpistemics(effectiveSessionId)
      .then((d) => {
        if (d && Object.keys(d).length > 0) setData(d as unknown as EpistemicResponse);
      })
      .catch((err) => console.error('Failed to load epistemics data:', err))
      .finally(() => setEpistemicsLoading(false));
  }, [effectiveSessionId]);

  const liveClaim = campaign?.epistemics ? {
    claim_id: `claim-${campaign.target_system.toLowerCase().replace(/[^a-z0-9]/g, '')}`,
    record_id: (data?.active_wavefunctions?.[0] as EpistemicWavefunction | undefined)?.record_id,
    statement: campaign.epistemics.statement,
    alpha_corr: campaign.epistemics.alpha_corroborated,
    beta_cont: campaign.epistemics.beta_contested,
    gamma_unv: campaign.epistemics.gamma_unverified,
    collapsed: campaign.epistemics.status === 'COLLAPSED_VERIFIED',
    status: campaign.epistemics.status,
  } : data?.active_wavefunctions?.[0];
  const hasLive = Boolean(liveClaim && (liveClaim.record_id || liveClaim.status !== 'UNVERIFIED'));
  const active = liveClaim ?? {
    claim_id: 'none',
    statement: 'No evaluated epistemic claim for this session.',
    alpha_corr: Number.NaN,
    beta_cont: Number.NaN,
    gamma_unv: Number.NaN,
    collapsed: false,
    status: 'UNVERIFIED',
  };

  // Verified chrome only on explicit collapse: high amplitudes alone
  // (e.g. template projections) must never render verified.
  const isVerified = hasLive && active.collapsed === true;
  const alpha = active.alpha_corr;
  const beta = active.beta_cont;
  const gamma = active.gamma_unv;
  const alphaNum = typeof alpha === 'number' && Number.isFinite(alpha) ? alpha : null;
  const betaNum = typeof beta === 'number' && Number.isFinite(beta) ? beta : null;
  const gammaNum = typeof gamma === 'number' && Number.isFinite(gamma) ? gamma : null;
  const discrepancy = campaign?.classical_anchor?.discrepancy_mha;
  const accuracyLimit = campaign?.classical_anchor?.chemical_accuracy_limit ?? 1.6;
  const anchorMethod = campaign?.classical_anchor?.method;
  const anchorComputed = campaign?.classical_anchor?.anchor_computed === true;
  const anchorStated =
    anchorComputed &&
    typeof discrepancy === 'number' &&
    Number.isFinite(discrepancy) &&
    Boolean(anchorMethod) &&
    anchorMethod !== 'uncomputed';
  const epistemicSpec: ChartSpec | undefined =
    hasLive && alphaNum != null && betaNum != null && gammaNum != null
      ? {
          kind: 'bars',
          title: 'Epistemic state probabilities',
          series: [
            {
              name: 'Probability',
              points: [
                { x: 'α corroborated', y: alphaNum * alphaNum, display: (alphaNum * alphaNum).toFixed(4) },
                { x: 'β contested', y: betaNum * betaNum, display: (betaNum * betaNum).toFixed(4) },
                { x: 'γ unverified', y: gammaNum * gammaNum, display: (gammaNum * gammaNum).toFixed(4) },
              ],
            },
          ],
          x: { label: 'Component', kind: 'category' },
          y: { label: 'Probability mass', format: 'number', min: 0, max: 1 },
          headline: { label: 'α²', value: (alphaNum * alphaNum).toFixed(4) },
          basis: `Squared amplitudes from ${active.record_id ? `claim record ${active.record_id}` : 'the live campaign projection'}; not a fresh measurement of truth.`,
          detail: 'full',
        }
      : undefined;

  return (
    <div data-provenance={hasLive ? 'measured' : undefined} className="relative min-h-[640px] rounded-3xl overflow-hidden bg-[var(--color-page)] border border-white/[0.08] shadow-2xl flex flex-col justify-between p-8 select-none">
      {/* Ambient Radial Backlight */}
      <div 
        className="absolute inset-0 pointer-events-none opacity-40 transition-opacity duration-1000"
        style={{
          background: isVerified
            ? 'radial-gradient(circle 500px at 50% 45%, rgba(16, 185, 129, 0.15), transparent 70%)'
            : 'radial-gradient(circle 500px at 50% 45%, rgba(245, 158, 11, 0.15), transparent 70%)',
        }}
      />

      {/* Top Header */}
      <div className="relative z-10 flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-2xl bg-white/[0.05] border border-white/10 flex items-center justify-center shadow-inner">
            <Compass className="w-5 h-5 text-emerald-400" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-base font-semibold text-white tracking-tight">Epistemic Truth Gauge</h2>
              {hasLive && <ProvenanceBadge kind="measured" detail="Evaluated claim record or live campaign epistemics." />}
            </div>
            <p className="text-xs text-slate-400 mt-0.5">
              Continuous credibility collapse against classical anchors &amp; quantum evidence
            </p>
          </div>
        </div>

        <div className={`flex items-center space-x-2 px-3.5 py-1.5 rounded-full bg-black/40 border ${isVerified ? 'border-white/10' : 'border-amber-400/40'}`}>
          <ShieldCheck className={`w-4 h-4 ${isVerified ? 'text-emerald-400' : 'text-amber-400'}`} />
          <span className={`text-xs font-mono font-medium ${isVerified ? 'text-emerald-300' : 'text-amber-300'}`}>
            {isVerified ? 'Ring 1 Epistemic Gate Active' : 'Gate unattested — no live claim'}
          </span>
        </div>
      </div>

      {/* Center Stage: The Living Epistemic Truth Orb */}
      <div className="relative z-10 flex-1 flex flex-col items-center justify-center my-8">
        {/* The Radiant Orb */}
        <div
          className="relative w-64 h-64 flex items-center justify-center group"
          title="Deep mathematical evidence below"
        >
          {/* Outer Waveform Rings */}
          <div 
            className={`absolute inset-0 rounded-full border border-dashed transition-all duration-1000 ${
              isVerified ? 'border-emerald-500/40' : 'border-amber-500/40'
            }`} 
          />
          <div 
            className={`absolute inset-4 rounded-full border border-white/10 transition-all duration-700 ${
              isVerified ? 'group-hover:border-emerald-400/50' : 'group-hover:border-amber-400/50'
            }`} 
          />

          {/* Central Pulsing Sphere */}
          <div 
            className={`w-40 h-40 rounded-full transition-all duration-700 flex flex-col items-center justify-center text-center p-4 shadow-2xl relative ${
              isVerified
                ? 'bg-gradient-to-tr from-emerald-600/80 via-teal-500/70 to-emerald-400/90 text-white'
                : 'bg-gradient-to-tr from-amber-600/80 via-orange-500/70 to-amber-400/90 text-white'
            }`}
          >
            {/* H4: the headline % is the corroborated amplitude mass α (not the
                squared probability α² shown in the deep-math panel below).
                Labelling the basis keeps the number while disambiguating it —
                coherent because both views now name which power of α they show. */}
            <div className="text-[12px] uppercase font-bold tracking-widest opacity-80">
              Confidence · α mass
            </div>
            <div className="text-3xl font-extrabold tracking-tight font-mono mt-0.5">
              {hasLive ? `${Math.round(alpha * 100)}%` : '—'}
            </div>
            <div className="text-[12px] font-medium opacity-90 mt-1">
              {hasLive ? (isVerified ? 'Verified True' : 'Evaluating') : 'Unverified'}
            </div>
            {!hasLive && !epistemicsLoading && (
              <div className="mt-1 px-2 py-0.5 rounded-full bg-white/10 border border-white/20 text-[12px] font-mono text-slate-300">
                no live claim — not session data
              </div>
            )}
            {!hasLive && epistemicsLoading && (
              <div className="mt-1 px-2 py-0.5 rounded-full bg-white/10 border border-white/20 text-[12px] font-mono text-slate-300 animate-pulse">
                Loading epistemics…
              </div>
            )}
          </div>
        </div>

        {/* Clear Plain-English Verdict */}
        <div className="max-w-md text-center mt-6 space-y-2">
          <div className="inline-flex items-center space-x-1.5 px-3 py-1 rounded-full bg-white/[0.08] border border-white/15 text-xs text-slate-200">
            <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" />
            <span>{hasLive && isVerified ? 'FCI Ground State Energy Verified' : 'No verified claim — run the swarm'}</span>
          </div>
          <h3 className="text-sm font-semibold text-white">
            &ldquo;{active.statement}&rdquo;
          </h3>
          {/* Claim record: the evaluated verdict behind this statement, when one exists. */}
          {active.record_id ? (
            <p className="text-[12px] font-mono text-cyan-300/80">record {active.record_id} · resolvable under session claims</p>
          ) : (
            <p className="text-[12px] text-slate-500">No evaluated record yet.</p>
          )}
          <p className="text-xs text-slate-300 leading-relaxed">
            {anchorStated ? (
              discrepancy! <= accuracyLimit ? (
                <>Corroborated within chemical accuracy (<span className="text-emerald-400 font-mono font-semibold">{discrepancy!.toFixed(2)} mHa &le; {typeof accuracyLimit === 'number' && Number.isFinite(accuracyLimit) ? accuracyLimit.toFixed(1) : '—'} mHa</span>) against {anchorMethod} benchmarks.</>
              ) : (
                <>Outside chemical accuracy (<span className="text-amber-400 font-mono font-semibold">{discrepancy!.toFixed(2)} mHa &gt; {typeof accuracyLimit === 'number' && Number.isFinite(accuracyLimit) ? accuracyLimit.toFixed(1) : '—'} mHa</span>) against {anchorMethod} benchmarks.</>
              )
            ) : (
              <>No anchor measured for this session — corroboration unattested.</>
            )}
          </p>
          {!hasLive && epistemicsLoading && (
            <p className="text-[12px] font-code text-slate-400 animate-pulse">Loading epistemics…</p>
          )}
        </div>
        {epistemicSpec && (
          <div className="mt-6 w-full max-w-2xl rounded-2xl border border-white/10 bg-black/30 p-4">
            <Chart spec={epistemicSpec} />
          </div>
        )}
      </div>

      {/* Progressive Disclosure (button reveals deep math; the orb itself is display-only) */}
      <div className="relative z-10 border-t border-white/[0.08] pt-4">
        <button
          onClick={() => setShowDeepMath(!showDeepMath)}
          className="w-full flex items-center justify-between text-xs text-slate-300 hover:text-white transition-colors py-1 font-medium"
        >
          <span className="flex items-center space-x-2 font-medium">
            <Scale className="w-3.5 h-3.5 text-cyan-400" />
            <span>{showDeepMath ? 'Hide Epistemic Wavefunction Mathematics' : 'Inspect Tripartite Wavefunction & Classical Anchors'}</span>
          </span>
          {showDeepMath ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
        </button>

        {showDeepMath && (
          <div className="mt-4 p-5 rounded-2xl bg-black/60 border border-white/10 grid grid-cols-1 md:grid-cols-3 gap-4 text-xs font-mono">
            <div className="space-y-1">
              <span className="text-slate-300 font-semibold">Corroborated (α):</span>
              <div className="text-base text-emerald-400 font-bold">{hasLive && alphaNum != null ? (alphaNum * alphaNum).toFixed(4) : '—'}</div>
              <p className="text-xs text-slate-300 font-sans">Probability mass in agreement with physical anchor</p>
            </div>

            <div className="space-y-1">
              <span className="text-slate-300 font-semibold">Contested (β):</span>
              <div className="text-base text-amber-400 font-bold">{hasLive && betaNum != null ? (betaNum * betaNum).toFixed(4) : '—'}</div>
              <p className="text-xs text-slate-300 font-sans">Active disconfirmation signal from tribunal</p>
            </div>

            <div className="space-y-1">
              <span className="text-slate-300 font-semibold">Unverified (γ):</span>
              <div className="text-base text-slate-200 font-bold">{hasLive && gammaNum != null ? (gammaNum * gammaNum).toFixed(4) : '—'}</div>
              <p className="text-xs text-slate-300 font-sans">Remaining epistemic uncertainty interval</p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
