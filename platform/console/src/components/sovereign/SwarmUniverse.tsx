'use client';

import React, { useEffect, useState } from 'react';
import { Brain, ShieldCheck, CircuitBoard, Scale } from 'lucide-react';
import { CampaignState, ProbeId } from '@/types';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';

interface SwarmUniverseProps {
  campaign?: CampaignState | null;
  recenterSignal?: number;
  activeProbe?: ProbeId | null;
  /**
   * Rest state: no run, no stream, no pending decision. Motion exists only
   * while its backing signal is live (ui-ux-vision §6: motion reserved for
   * state transitions) — at rest the sweep parks, orbits freeze, beams dim
   * to hairlines. Anything animating at rest is a bug, not ambience.
   */
  atRest?: boolean;
}

function shortBackend(name: string): string {
  const parts = name.split(/\s+/).filter(Boolean);
  return parts.slice(0, 2).join(' ') || 'Local simulator';
}

function HoverCard({
  accent,
  title,
  body,
  rows,
  forceOpen = false,
}: {
  accent: 'amber' | 'emerald' | 'cyan' | 'rose';
  title: string;
  body: string;
  rows: Array<[string, string, string?]>;
  forceOpen?: boolean;
}) {
  const border =
    accent === 'amber'
      ? 'border-[var(--color-warning-edge)]'
      : accent === 'emerald'
        ? 'border-[var(--color-success-edge)]'
        : accent === 'cyan'
          ? 'border-[var(--color-info-edge)]'
          : 'border-[var(--color-danger-edge)]';
  const text =
    accent === 'amber'
      ? 'text-[var(--color-warning-ink)]'
      : accent === 'emerald'
        ? 'text-[var(--color-success-ink)]'
        : accent === 'cyan'
          ? 'text-[var(--color-info-ink)]'
          : 'text-[var(--color-danger-ink)]';
  return (
    <div
      onClick={(e) => e.stopPropagation()}
      className={`absolute top-12 left-0 w-64 p-3 rounded-xl bg-[var(--color-surface-elev)] border ${border} shadow-[var(--shadow-float)] transition-all duration-200 z-50 ${
        forceOpen ? 'opacity-100 pointer-events-auto' : 'opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto'
      }`}
    >
      <div className={`text-[12px] uppercase font-code ${text} tracking-wider`}>{title}</div>
      <p className="text-xs text-[var(--color-label-secondary)] mt-1 leading-snug">{body}</p>
      <div className="mt-2 pt-2 border-t border-[var(--color-border)] space-y-1">
        {rows.map(([k, v, vc]) => (
          <div key={k} className="flex justify-between text-[12px] font-code text-[var(--color-muted)]">
            <span>{k}</span>
            <span className={vc || 'text-[var(--color-text)]'}>{v}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

export function SwarmUniverse({ campaign, recenterSignal = 0, activeProbe = null, atRest = false }: SwarmUniverseProps) {
  // Tap-to-open (keyboard/mouse/touch): hover alone excluded touch and
  // keyboard users. Cards open on hover (existing CSS) OR pinned open by
  // tap; Esc or re-tap closes. One open at a time (D1: summary first).
  const [openCard, setOpenCard] = useState<string | null>(null);
  useEffect(() => {
    if (openCard === null || typeof window === 'undefined') return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpenCard(null);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [openCard]);
  const toggleCard = (id: string) => setOpenCard((cur) => (cur === id ? null : id));
  const qf = campaign?.quantum_factors;
  const anchor = campaign?.classical_anchor;
  // No-campaign empty state: no invented H2 default. Without a campaign the
  // arena states that it is empty — target, energy, and probes stay silent.
  const hasCampaign = campaign != null;
  const targetLabel = qf?.target_molecule || campaign?.target_system || (hasCampaign ? '—' : 'No campaign — swarm idle');
  const energyRaw = anchor?.anchor_computed === true
    ? (anchor.vqe_expectation_energy ?? anchor.fci_exact_energy)
    : undefined;
  const energyText = typeof energyRaw === 'number' && Number.isFinite(energyRaw) ? energyRaw.toFixed(4) : '—';

  const gradVar = typeof qf?.gradient_variance === 'number' && Number.isFinite(qf.gradient_variance) ? qf.gradient_variance : null;
  const bpRisk = (qf?.barren_plateau_risk || (hasCampaign ? 'unknown' : '—')).toUpperCase();
  // Critic verdict hides without live factors: no qf means no risk signal,
  // never a default WATCHING/CLEARED.
  const criticCleared = qf ? bpRisk.includes('LOW') : false;
  const criticState = !qf ? '—' : criticCleared ? 'CLEARED' : 'WATCHING';
  const epistemics = campaign?.epistemics;
  const pCorr = typeof epistemics?.p_correlated === 'number' && Number.isFinite(epistemics.p_correlated) ? epistemics.p_correlated : null;
  const litDrift = typeof epistemics?.literature_drift_mha === 'number' && Number.isFinite(epistemics.literature_drift_mha) ? epistemics.literature_drift_mha : null;
  const meyerWallach = typeof qf?.meyer_wallach_q === 'number' && Number.isFinite(qf.meyer_wallach_q) ? qf.meyer_wallach_q : null;
  const estSpend = typeof qf?.estimated_spend_usd === 'number' && Number.isFinite(qf.estimated_spend_usd) ? qf.estimated_spend_usd : null;
  const canary = typeof qf?.clifford_fidelity === 'number' && Number.isFinite(qf.clifford_fidelity) ? qf.clifford_fidelity : null;
  // Audit 2026-10-21: the 0.984 canary is a synthesis-template constant
  // (campaign_synth hardcodes it), not a measured probe. Until a job fold
  // collapses the campaign (COLLAPSED_VERIFIED), the pill says template.
  const canaryVerified = campaign?.epistemics?.status === 'COLLAPSED_VERIFIED';
  const canaryLabel = canary != null ? (canaryVerified ? `canary ${canary.toFixed(3)}` : `canary ${canary.toFixed(3)} (template)`) : 'canary pending';

  return (
    <div
      data-testid="swarm-universe"
      data-provenance={anchor?.anchor_computed === true ? 'measured' : hasCampaign ? 'proposed' : undefined}
      data-rest={atRest ? 'true' : undefined}
      className="absolute inset-0 flex items-center justify-center cursor-crosshair"
    >
      {/* Coordinate rings + radar sweep (parked at rest) */}
      <div key={recenterSignal} className="absolute inset-0 flex items-center justify-center pointer-events-none">
        <div className={`absolute w-[720px] h-[720px] max-w-[95vw] max-h-[95vw] rounded-full border border-[var(--color-info-edge)] opacity-50 ${atRest ? '' : 'animate-orbit-slow'}`} />
        <div className={`absolute w-[520px] h-[520px] max-w-[75vw] max-h-[75vw] rounded-full border border-dashed border-[var(--color-border)] ${atRest ? '' : 'animate-orbit-reverse'}`} />
        <div className="absolute w-[340px] h-[340px] max-w-[55vw] max-h-[55vw] rounded-full border border-[var(--color-warning-edge)] opacity-50" />
        {!atRest && <div className="absolute w-[720px] h-[720px] max-w-[95vw] max-h-[95vw] rounded-full animate-sweep" />}
      </div>

      {/* Resonance beams */}
      <svg className="absolute inset-0 w-full h-full pointer-events-none" aria-hidden="true">
        <defs>
          <linearGradient id="beamTheorist" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="var(--color-warning)" stopOpacity="0.8" />
            <stop offset="100%" stopColor="var(--color-info)" stopOpacity="0.15" />
          </linearGradient>
          <linearGradient id="beamFormalist" x1="100%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" stopColor="var(--color-success)" stopOpacity="0.8" />
            <stop offset="100%" stopColor="var(--color-info)" stopOpacity="0.15" />
          </linearGradient>
          <linearGradient id="beamCompiler" x1="0%" y1="100%" x2="100%" y2="0%">
            <stop offset="0%" stopColor="var(--color-info)" stopOpacity="0.8" />
            <stop offset="100%" stopColor="var(--color-info)" stopOpacity="0.15" />
          </linearGradient>
          <linearGradient id="beamCritic" x1="100%" y1="100%" x2="0%" y2="0%">
            <stop offset="0%" stopColor="var(--color-danger)" stopOpacity="0.8" />
            <stop offset="100%" stopColor="var(--color-info)" stopOpacity="0.15" />
          </linearGradient>
        </defs>
        <line x1="32%" y1="36%" x2="48%" y2="48%" stroke="url(#beamTheorist)" strokeWidth="1.5" strokeDasharray="4 4" className={atRest ? 'opacity-20' : 'opacity-70 animate-dash-drift'} />
        <line x1="66%" y1="32%" x2="52%" y2="48%" stroke="url(#beamFormalist)" strokeWidth="1.5" className={atRest ? 'opacity-20' : 'opacity-80'} />
        <line x1="68%" y1="65%" x2="52%" y2="52%" stroke="url(#beamCompiler)" strokeWidth="1.5" strokeDasharray="6 3" className={atRest ? 'opacity-20' : 'opacity-80 animate-dash-drift'} />
        <line x1="34%" y1="68%" x2="48%" y2="52%" stroke="url(#beamCritic)" strokeWidth="1" className={atRest ? 'opacity-10' : 'opacity-50'} />
      </svg>

      {/* Central target singularity (display-only: numbers already visible) */}
      <div className="relative z-10 flex flex-col items-center">
        <div className={`w-36 h-36 rounded-full bg-gradient-to-tr from-[color-mix(in_srgb,var(--color-info)_20%,transparent)] via-[color-mix(in_srgb,var(--color-warning)_10%,transparent)] to-transparent border border-[color-mix(in_srgb,var(--color-info)_35%,transparent)] flex items-center justify-center ${atRest ? '' : 'animate-orb'}`}>
          <div className="w-24 h-24 rounded-full bg-[var(--color-raised)] border border-[color-mix(in_srgb,var(--color-info)_60%,transparent)] flex items-center justify-center shadow-[var(--shadow-float)] relative">
            <svg width="44" height="44" viewBox="0 0 24 24" fill="none" className="text-[var(--color-info)]">
              <path d="M12 2L2 7L12 12L22 7L12 2Z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
              <path d="M2 17L12 22L22 17" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
              <path d="M2 12L12 17L22 12" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
              <circle cx="12" cy="12" r="2" fill="var(--color-warning)" />
            </svg>
            {!atRest && <div className="absolute -top-1 right-2 w-2 h-2 rounded-full bg-[var(--color-warning)] animate-ping" />}
          </div>
        </div>
        <div className="mt-3 flex flex-col items-center">
          <div className="flex items-center gap-1.5">
            <div className="px-2.5 py-0.5 rounded-full bg-[var(--color-info-wash)] border border-[var(--color-info-edge)] text-[12px] font-code text-[var(--color-info-ink)] tracking-wider max-w-[220px] truncate">
              {targetLabel}
            </div>
            {hasCampaign && <ProvenanceBadge kind={anchor?.anchor_computed === true ? 'measured' : 'proposed'} detail={anchor?.anchor_computed === true ? 'Computed anchor and campaign state.' : 'Campaign proposal; no measured anchor yet.'} />}
          </div>
          <div data-testid="universe-energy" className="font-code text-sm text-[var(--color-text)] font-semibold mt-1">
            {energyText} <span className="text-xs font-normal text-[var(--color-muted)]">Hartree</span>
          </div>
          {atRest && (
            <div data-testid="universe-rest-note" className="font-code text-[12px] text-[var(--color-faint)] mt-1">
              idle · at rest
            </div>
          )}
        </div>
      </div>

      {/* Probe α · Theorist */}
      <button data-testid="probe-theorist" data-active={activeProbe === 'theorist' ? 'true' : undefined} style={{ animationDelay: '0s' }} onClick={() => toggleCard('theorist')} aria-expanded={openCard === 'theorist'} aria-label="Theorist probe details" className={`absolute ${atRest ? "" : "animate-float-gentle"} top-[32%] left-[28%] group cursor-pointer z-10 transition-transform duration-300 hover:scale-110 ${activeProbe === 'theorist' ? 'scale-110' : ''} bg-transparent border-0 p-0 text-left`}>
        <div className="relative flex items-center gap-2">
          <div className="w-9 h-9 rounded-xl bg-[var(--color-surface-elev)] border border-[var(--color-warning-edge)] flex items-center justify-center shadow-[var(--shadow-card)] group-hover:border-[var(--color-warning)]">
            <Brain className="w-[18px] h-[18px] text-[var(--color-warning-ink)]" />
          </div>
          <div className="bg-[var(--color-surface)] border border-[var(--color-border-strong)] rounded-lg px-2.5 py-1 flex flex-col shadow-[var(--shadow-card)]">
            <div className="flex items-center gap-1.5">
              <span className="w-1.5 h-1.5 rounded-full bg-[var(--color-warning)]" />
              <span className="text-[12px] font-bold text-[var(--color-text)] tracking-wide">Theorist</span>
              <span className="text-[12px] font-code text-[var(--color-warning-ink)] ml-1">ORBITING</span>
            </div>
            <div className="text-[12px] font-code text-[var(--color-muted)]">
              {qf ? `${qf.active_electrons}e/${qf.active_orbitals}o · ${qf.ansatz_family}` : 'ansatz uncharted'}
            </div>
          </div>
        </div>
        <HoverCard
          accent="amber"
          title="Live Agent Telemetry"
          body={qf ? `Exploring ${targetLabel} under ${qf.ansatz_family} (${qf.fermion_mapping}).` : 'Awaiting campaign state.'}
          rows={[
            ['∇ variance', gradVar != null ? gradVar.toExponential(2) : '—', 'text-[var(--color-text)]'],
            ['Entanglement Q', meyerWallach != null ? meyerWallach.toFixed(3) : '—', 'text-[var(--color-text)]'],
            ['BP risk', qf ? bpRisk : '—', qf ? (criticCleared ? 'text-[var(--color-success-ink)]' : 'text-[var(--color-warning-ink)]') : 'text-[var(--color-text)]'],
          ]}
          forceOpen={openCard === 'theorist'}
        />
      </button>

      {/* Probe β · Formalist */}
      <button data-testid="probe-formalist" data-active={activeProbe === 'formalist' ? 'true' : undefined} style={{ animationDelay: '0.9s' }} onClick={() => toggleCard('formalist')} aria-expanded={openCard === 'formalist'} aria-label="Formalist probe details" className={`absolute ${atRest ? "" : "animate-float-gentle"} top-[28%] right-[30%] group cursor-pointer z-10 transition-transform duration-300 hover:scale-110 ${activeProbe === 'formalist' ? 'scale-110' : ''} bg-transparent border-0 p-0 text-left`}>
        <div className="relative flex items-center gap-2">
          <div className="bg-[var(--color-surface)] border border-[var(--color-border-strong)] rounded-lg px-2.5 py-1 flex flex-col text-right shadow-[var(--shadow-card)]">
            <div className="flex items-center justify-end gap-1.5">
              <span className="text-[12px] font-code text-[var(--color-success-ink)]">
                {pCorr != null ? `ρ ${pCorr.toFixed(3)}` : 'ρ —'}
              </span>
              <span className="text-[12px] font-bold text-[var(--color-text)] tracking-wide">Formalist</span>
              <span className="w-1.5 h-1.5 rounded-full bg-[var(--color-success)]" />
            </div>
            <div className="text-[12px] font-code text-[var(--color-muted)]">
              {campaign?.epistemics?.status || (hasCampaign ? 'Epistemic status' : 'No campaign')}
            </div>
          </div>
          <div className="w-9 h-9 rounded-xl bg-[var(--color-surface-elev)] border border-[var(--color-success-edge)] flex items-center justify-center shadow-[var(--shadow-card)] group-hover:border-[var(--color-success)]">
            <ShieldCheck className="w-[18px] h-[18px] text-[var(--color-success-ink)]" />
          </div>
        </div>
        <HoverCard
          accent="emerald"
          title="Belief State"
          body={campaign?.epistemics?.statement || 'Awaiting epistemic state.'}
          rows={[
            ['Corroborated α', `${campaign?.epistemics?.alpha_corroborated ?? '—'}`],
            ['Contested β', `${campaign?.epistemics?.beta_contested ?? '—'}`],
            ['Literature drift', litDrift != null ? `${litDrift.toFixed(2)} mHa` : '—'],
          ]}
          forceOpen={openCard === 'formalist'}
        />
      </button>

      {/* Probe γ · Compiler */}
      <button data-testid="probe-compiler" data-active={activeProbe === 'compiler' ? 'true' : undefined} style={{ animationDelay: '1.8s' }} onClick={() => toggleCard('compiler')} aria-expanded={openCard === 'compiler'} aria-label="Compiler probe details" className={`absolute ${atRest ? "" : "animate-float-gentle"} bottom-[30%] right-[28%] group cursor-pointer z-10 transition-transform duration-300 hover:scale-110 ${activeProbe === 'compiler' ? 'scale-110' : ''} bg-transparent border-0 p-0 text-left`}>
        <div className="relative flex items-center gap-2">
          <div className="bg-[var(--color-surface)] border border-[var(--color-border-strong)] rounded-lg px-2.5 py-1 flex flex-col text-right shadow-[var(--shadow-card)]">
            <div className="flex items-center justify-end gap-1.5">
              <span className="text-[12px] font-code text-[var(--color-info-ink)]">
                {qf ? `${qf.qubit_count} Qubits` : '— Qubits'}
              </span>
              <span className="text-[12px] font-bold text-[var(--color-text)] tracking-wide">Compiler</span>
              <span className="w-1.5 h-1.5 rounded-full bg-[var(--color-info)]" />
            </div>
            <div className="text-[12px] font-code text-[var(--color-muted)]">
              {qf ? shortBackend(qf.recommended_backend) : 'backend uncharted'}
            </div>
          </div>
          <div className="w-9 h-9 rounded-xl bg-[var(--color-surface-elev)] border border-[var(--color-info-edge)] flex items-center justify-center shadow-[var(--shadow-card)] group-hover:border-[var(--color-info)]">
            <CircuitBoard className="w-[18px] h-[18px] text-[var(--color-info-ink)]" />
          </div>
        </div>
        <HoverCard
          accent="cyan"
          title="Hardware Transpilation"
          body={qf ? `Depth ${qf.circuit_depth} · ${qf.two_qubit_gates} two-qubit gates · ${qf.fermion_mapping}.` : 'Awaiting circuit.'}
          rows={[
            ['Est. spend', estSpend != null ? `$${estSpend.toFixed(2)}` : '—'],
            ['Canary fidelity', canary != null ? (canaryVerified ? canary.toFixed(4) : `${canary.toFixed(4)} (template)`) : '—'],
          ]}
          forceOpen={openCard === 'compiler'}
        />
      </button>

      {/* Probe δ · Critic */}
      <button data-testid="probe-critic" data-active={activeProbe === 'critic' ? 'true' : undefined} style={{ animationDelay: '2.7s' }} onClick={() => toggleCard('critic')} aria-expanded={openCard === 'critic'} aria-label="Critic probe details" className={`absolute ${atRest ? "" : "animate-float-gentle"} bottom-[28%] left-[30%] group cursor-pointer z-10 transition-transform duration-300 hover:scale-110 ${activeProbe === 'critic' ? 'scale-110' : ''} bg-transparent border-0 p-0 text-left`}>
        <div className="relative flex items-center gap-2">
          <div className="w-9 h-9 rounded-xl bg-[var(--color-surface-elev)] border border-[var(--color-danger-edge)] flex items-center justify-center shadow-[var(--shadow-card)] group-hover:border-[var(--color-danger)]">
            <Scale className="w-[18px] h-[18px] text-[var(--color-danger-ink)]" />
          </div>
          <div className="bg-[var(--color-surface)] border border-[var(--color-border-strong)] rounded-lg px-2.5 py-1 flex flex-col shadow-[var(--shadow-card)]">
            <div className="flex items-center gap-1.5">
              <span className="w-1.5 h-1.5 rounded-full bg-[var(--color-danger)]" />
              <span className="text-[12px] font-bold text-[var(--color-text)] tracking-wide">Critic</span>
              <span className="text-[12px] font-code text-[var(--color-danger-ink)] ml-1">
                {criticState}
              </span>
            </div>
            <div className="text-[12px] font-code text-[var(--color-muted)]">
              {canaryLabel}
            </div>
          </div>
        </div>
        <HoverCard
          accent="rose"
          title="Trap Watch"
          body={!qf ? 'Awaiting campaign — no barren-plateau signal.' : criticCleared ? 'Barren-plateau risk low: variational path clear.' : `Barren-plateau risk ${bpRisk.toLowerCase()}: holding dispatch.`}
          rows={[
            ['BP risk', qf ? bpRisk : '—', qf ? (criticCleared ? 'text-[var(--color-success-ink)]' : 'text-[var(--color-danger-ink)]') : 'text-[var(--color-text)]'],
            ['∇ variance', gradVar != null ? gradVar.toExponential(2) : '—', 'text-[var(--color-text)]'],
          ]}
          forceOpen={openCard === 'critic'}
        />
      </button>
    </div>
  );
}
