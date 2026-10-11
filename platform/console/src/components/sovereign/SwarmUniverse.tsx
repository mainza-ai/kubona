'use client';

import React from 'react';
import { Brain, ShieldCheck, CircuitBoard, Scale } from 'lucide-react';
import { CampaignState, ProbeId } from '@/types';

interface SwarmUniverseProps {
  campaign?: CampaignState | null;
  recenterSignal?: number;
  activeProbe?: ProbeId | null;
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
}: {
  accent: 'amber' | 'emerald' | 'cyan' | 'rose';
  title: string;
  body: string;
  rows: Array<[string, string, string?]>;
}) {
  const border =
    accent === 'amber'
      ? 'border-amber-400/30'
      : accent === 'emerald'
        ? 'border-emerald-400/30'
        : accent === 'cyan'
          ? 'border-cyan-400/30'
          : 'border-rose-400/30';
  const text =
    accent === 'amber'
      ? 'text-amber-400'
      : accent === 'emerald'
        ? 'text-emerald-400'
        : accent === 'cyan'
          ? 'text-cyan-400'
          : 'text-rose-400';
  return (
    <div
      className={`absolute top-12 left-0 w-64 p-3 rounded-xl bg-[#0D1017]/95 border ${border} backdrop-blur-2xl shadow-2xl opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto transition-all duration-200 z-50`}
    >
      <div className={`text-[10px] uppercase font-code ${text} tracking-wider`}>{title}</div>
      <p className="text-xs text-slate-200 mt-1 leading-snug">{body}</p>
      <div className="mt-2 pt-2 border-t border-white/10 space-y-1">
        {rows.map(([k, v, vc]) => (
          <div key={k} className="flex justify-between text-[10px] font-code text-slate-400">
            <span>{k}</span>
            <span className={vc || 'text-white'}>{v}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

export function SwarmUniverse({ campaign, recenterSignal = 0, activeProbe = null }: SwarmUniverseProps) {
  const qf = campaign?.quantum_factors;
  const anchor = campaign?.classical_anchor;

  const targetLabel = qf?.target_molecule || campaign?.target_system || 'H2';
  const energy = anchor?.vqe_expectation_energy ?? anchor?.fci_exact_energy;
  const energyText = energy != null && Number.isFinite(energy) ? energy.toFixed(4) : '—';

  const gradVar = qf?.gradient_variance;
  const bpRisk = (qf?.barren_plateau_risk || 'unknown').toUpperCase();
  const criticCleared = bpRisk.includes('LOW');

  return (
    <div
      data-testid="swarm-universe"
      className="absolute inset-0 flex items-center justify-center cursor-crosshair"
    >
      {/* Coordinate rings + radar sweep */}
      <div key={recenterSignal} className="absolute inset-0 flex items-center justify-center pointer-events-none">
        <div className="absolute w-[720px] h-[720px] max-w-[95vw] max-h-[95vw] rounded-full border border-cyan-500/10 animate-orbit-slow" />
        <div className="absolute w-[520px] h-[520px] max-w-[75vw] max-h-[75vw] rounded-full border border-dashed border-white/5 animate-orbit-reverse" />
        <div className="absolute w-[340px] h-[340px] max-w-[55vw] max-h-[55vw] rounded-full border border-amber-500/10" />
        <div className="absolute w-[720px] h-[720px] max-w-[95vw] max-h-[95vw] rounded-full animate-sweep" />
      </div>

      {/* Resonance beams */}
      <svg className="absolute inset-0 w-full h-full pointer-events-none" aria-hidden="true">
        <defs>
          <linearGradient id="beamTheorist" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#FFB300" stopOpacity="0.8" />
            <stop offset="100%" stopColor="#00E5FF" stopOpacity="0.1" />
          </linearGradient>
          <linearGradient id="beamFormalist" x1="100%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" stopColor="#00E676" stopOpacity="0.8" />
            <stop offset="100%" stopColor="#00E5FF" stopOpacity="0.1" />
          </linearGradient>
          <linearGradient id="beamCompiler" x1="0%" y1="100%" x2="100%" y2="0%">
            <stop offset="0%" stopColor="#00E5FF" stopOpacity="0.8" />
            <stop offset="100%" stopColor="#00E5FF" stopOpacity="0.1" />
          </linearGradient>
          <linearGradient id="beamCritic" x1="100%" y1="100%" x2="0%" y2="0%">
            <stop offset="0%" stopColor="#F43F5E" stopOpacity="0.8" />
            <stop offset="100%" stopColor="#00E5FF" stopOpacity="0.1" />
          </linearGradient>
        </defs>
        <line x1="32%" y1="36%" x2="48%" y2="48%" stroke="url(#beamTheorist)" strokeWidth="1.5" strokeDasharray="4 4" className="opacity-70 animate-dash-drift" />
        <line x1="66%" y1="32%" x2="52%" y2="48%" stroke="url(#beamFormalist)" strokeWidth="1.5" className="opacity-80" />
        <line x1="68%" y1="65%" x2="52%" y2="52%" stroke="url(#beamCompiler)" strokeWidth="1.5" strokeDasharray="6 3" className="opacity-80 animate-dash-drift" />
        <line x1="34%" y1="68%" x2="48%" y2="52%" stroke="url(#beamCritic)" strokeWidth="1" className="opacity-50" />
      </svg>

      {/* Central target singularity */}
      <div className="relative z-10 flex flex-col items-center group cursor-pointer">
        <div className="w-36 h-36 rounded-full bg-gradient-to-tr from-cyan-500/20 via-amber-500/10 to-transparent border border-cyan-400/30 flex items-center justify-center animate-orb">
          <div className="w-24 h-24 rounded-full bg-[#0F131C] border border-cyan-400/60 flex items-center justify-center shadow-[0_0_35px_-5px_rgba(0,229,255,0.25)] relative">
            <svg width="44" height="44" viewBox="0 0 24 24" fill="none" className="text-cyan-300">
              <path d="M12 2L2 7L12 12L22 7L12 2Z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
              <path d="M2 17L12 22L22 17" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
              <path d="M2 12L12 17L22 12" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
              <circle cx="12" cy="12" r="2" fill="#FFB300" />
            </svg>
            <div className="absolute -top-1 right-2 w-2 h-2 rounded-full bg-amber-400 animate-ping" />
          </div>
        </div>
        <div className="mt-3 flex flex-col items-center">
          <div className="px-2.5 py-0.5 rounded-full bg-cyan-950/80 border border-cyan-500/30 text-[10px] font-code text-cyan-300 tracking-wider max-w-[220px] truncate">
            {targetLabel}
          </div>
          <div data-testid="universe-energy" className="font-code text-sm text-white font-semibold mt-1">
            {energyText} <span className="text-xs font-normal text-slate-400">Hartree</span>
          </div>
        </div>
      </div>

      {/* Probe α · Theorist */}
      <div data-testid="probe-theorist" data-active={activeProbe === 'theorist' ? 'true' : undefined} style={{ animationDelay: '0s' }} className={`absolute animate-float-gentle top-[32%] left-[28%] group cursor-pointer z-10 transition-transform duration-300 hover:scale-110 ${activeProbe === 'theorist' ? 'scale-110' : ''}`}>
        <div className="relative flex items-center gap-2">
          <div className="w-9 h-9 rounded-xl bg-[#141822] border border-amber-400/50 flex items-center justify-center shadow-[0_0_30px_-5px_rgba(255,179,0,0.25)] group-hover:border-amber-400">
            <Brain className="w-[18px] h-[18px] text-amber-400" />
          </div>
          <div className="bg-[#0D1017]/90 backdrop-blur-md border border-white/10 rounded-lg px-2.5 py-1 flex flex-col">
            <div className="flex items-center gap-1.5">
              <span className="w-1.5 h-1.5 rounded-full bg-amber-400" />
              <span className="text-[11px] font-bold text-white tracking-wide">Theorist</span>
              <span className="text-[9px] font-code text-amber-400 ml-1">ORBITING</span>
            </div>
            <div className="text-[10px] font-code text-slate-400">
              {qf ? `${qf.active_electrons}e/${qf.active_orbitals}o · ${qf.ansatz_family}` : 'ansatz uncharted'}
            </div>
          </div>
        </div>
        <HoverCard
          accent="amber"
          title="Live Agent Telemetry"
          body={qf ? `Exploring ${targetLabel} under ${qf.ansatz_family} (${qf.fermion_mapping}).` : 'Awaiting campaign state.'}
          rows={[
            ['∇ variance', gradVar != null ? gradVar.toExponential(2) : '—', 'text-white'],
            ['Entanglement Q', qf?.meyer_wallach_q != null ? qf.meyer_wallach_q.toFixed(3) : '—', 'text-white'],
            ['BP risk', bpRisk, criticCleared ? 'text-emerald-400' : 'text-amber-400'],
          ]}
        />
      </div>

      {/* Probe β · Formalist */}
      <div data-testid="probe-formalist" data-active={activeProbe === 'formalist' ? 'true' : undefined} style={{ animationDelay: '0.9s' }} className={`absolute animate-float-gentle top-[28%] right-[30%] group cursor-pointer z-10 transition-transform duration-300 hover:scale-110 ${activeProbe === 'formalist' ? 'scale-110' : ''}`}>
        <div className="relative flex items-center gap-2">
          <div className="bg-[#0D1017]/90 backdrop-blur-md border border-white/10 rounded-lg px-2.5 py-1 flex flex-col text-right">
            <div className="flex items-center justify-end gap-1.5">
              <span className="text-[9px] font-code text-emerald-400">
                {campaign?.epistemics ? `ρ ${(campaign.epistemics.p_correlated ?? 0).toFixed(3)}` : 'ρ —'}
              </span>
              <span className="text-[11px] font-bold text-white tracking-wide">Formalist</span>
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-400" />
            </div>
            <div className="text-[10px] font-code text-slate-400">
              {campaign?.epistemics?.status || 'Epistemic status'}
            </div>
          </div>
          <div className="w-9 h-9 rounded-xl bg-[#141822] border border-emerald-400/50 flex items-center justify-center shadow-xl group-hover:border-emerald-400">
            <ShieldCheck className="w-[18px] h-[18px] text-emerald-400" />
          </div>
        </div>
        <HoverCard
          accent="emerald"
          title="Belief State"
          body={campaign?.epistemics?.statement || 'Awaiting epistemic state.'}
          rows={[
            ['Corroborated α', `${campaign?.epistemics?.alpha_corroborated ?? '—'}`],
            ['Contested β', `${campaign?.epistemics?.beta_contested ?? '—'}`],
            ['Literature drift', campaign?.epistemics ? `${campaign.epistemics.literature_drift_mha.toFixed(2)} mHa` : '—'],
          ]}
        />
      </div>

      {/* Probe γ · Compiler */}
      <div data-testid="probe-compiler" data-active={activeProbe === 'compiler' ? 'true' : undefined} style={{ animationDelay: '1.8s' }} className={`absolute animate-float-gentle bottom-[30%] right-[28%] group cursor-pointer z-10 transition-transform duration-300 hover:scale-110 ${activeProbe === 'compiler' ? 'scale-110' : ''}`}>
        <div className="relative flex items-center gap-2">
          <div className="bg-[#0D1017]/90 backdrop-blur-md border border-white/10 rounded-lg px-2.5 py-1 flex flex-col text-right">
            <div className="flex items-center justify-end gap-1.5">
              <span className="text-[9px] font-code text-cyan-400">
                {qf ? `${qf.qubit_count} Qubits` : '— Qubits'}
              </span>
              <span className="text-[11px] font-bold text-white tracking-wide">Compiler</span>
              <span className="w-1.5 h-1.5 rounded-full bg-cyan-400" />
            </div>
            <div className="text-[10px] font-code text-slate-400">
              {qf ? shortBackend(qf.recommended_backend) : 'backend uncharted'}
            </div>
          </div>
          <div className="w-9 h-9 rounded-xl bg-[#141822] border border-cyan-400/50 flex items-center justify-center shadow-[0_0_35px_-5px_rgba(0,229,255,0.25)] group-hover:border-cyan-400">
            <CircuitBoard className="w-[18px] h-[18px] text-cyan-400" />
          </div>
        </div>
        <HoverCard
          accent="cyan"
          title="Hardware Transpilation"
          body={qf ? `Depth ${qf.circuit_depth} · ${qf.two_qubit_gates} two-qubit gates · ${qf.fermion_mapping}.` : 'Awaiting circuit.'}
          rows={[
            ['Est. spend', qf ? `$${qf.estimated_spend_usd.toFixed(2)}` : '—'],
            ['Canary fidelity', qf?.clifford_fidelity != null ? qf.clifford_fidelity.toFixed(4) : '—'],
          ]}
        />
      </div>

      {/* Probe δ · Critic */}
      <div data-testid="probe-critic" data-active={activeProbe === 'critic' ? 'true' : undefined} style={{ animationDelay: '2.7s' }} className={`absolute animate-float-gentle bottom-[28%] left-[30%] group cursor-pointer z-10 transition-transform duration-300 hover:scale-110 ${activeProbe === 'critic' ? 'scale-110' : ''}`}>
        <div className="relative flex items-center gap-2">
          <div className="w-9 h-9 rounded-xl bg-[#141822] border border-rose-400/50 flex items-center justify-center shadow-xl group-hover:border-rose-400">
            <Scale className="w-[18px] h-[18px] text-rose-400" />
          </div>
          <div className="bg-[#0D1017]/90 backdrop-blur-md border border-white/10 rounded-lg px-2.5 py-1 flex flex-col">
            <div className="flex items-center gap-1.5">
              <span className="w-1.5 h-1.5 rounded-full bg-rose-400" />
              <span className="text-[11px] font-bold text-white tracking-wide">Critic</span>
              <span className="text-[9px] font-code text-rose-400 ml-1">
                {criticCleared ? 'CLEARED' : 'WATCHING'}
              </span>
            </div>
            <div className="text-[10px] font-code text-slate-400">
              {qf?.clifford_fidelity != null ? `canary ${qf.clifford_fidelity.toFixed(3)}` : 'canary pending'}
            </div>
          </div>
        </div>
        <HoverCard
          accent="rose"
          title="Trap Watch"
          body={criticCleared ? 'Barren-plateau risk low: variational path clear.' : `Barren-plateau risk ${bpRisk.toLowerCase()}: holding dispatch.`}
          rows={[
            ['BP risk', bpRisk, criticCleared ? 'text-emerald-400' : 'text-rose-400'],
            ['∇ variance', gradVar != null ? gradVar.toExponential(2) : '—', 'text-white'],
          ]}
        />
      </div>
    </div>
  );
}
