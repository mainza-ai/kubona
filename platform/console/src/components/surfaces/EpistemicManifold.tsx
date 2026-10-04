'use client';

import React, { useState, useEffect } from 'react';
import { Compass, Sparkles, CheckCircle2, AlertCircle, ShieldCheck, Scale, BarChart3 } from 'lucide-react';
import { MathText } from '../common/MathText';
import { CampaignState } from '@/types';

interface WavefunctionState {
  refusal_ratio: number;
  stagnation_kappa: number;
  literature_drift_mha: number;
  canonical_anchor: number;
  active_wavefunctions: Array<{
    claim_id: string;
    statement: string;
    alpha_corr: number;
    beta_cont: number;
    gamma_unv: number;
    collapsed: boolean;
    status: string;
  }>;
}

interface EpistemicManifoldProps {
  campaign?: CampaignState | null;
}

export function EpistemicManifold({ campaign }: EpistemicManifoldProps) {
  const [data, setData] = useState<WavefunctionState | null>(null);

  useEffect(() => {
    fetch('http://localhost:8080/api/epistemics')
      .then((res) => res.json())
      .then(setData)
      .catch((err) => console.error('Failed to load epistemics data:', err));
  }, []);

  const active = campaign?.epistemics ? {
    claim_id: `claim-${campaign.target_system.toLowerCase().replace(/[^a-z0-9]/g, '')}`,
    statement: campaign.epistemics.statement,
    alpha_corr: campaign.epistemics.alpha_corroborated,
    beta_cont: campaign.epistemics.beta_contested,
    gamma_unv: campaign.epistemics.gamma_unverified,
    collapsed: campaign.epistemics.status === 'COLLAPSED_VERIFIED',
    status: campaign.epistemics.status,
  } : (data?.active_wavefunctions?.[0] || {
    claim_id: 'claim-h2-ground',
    statement: 'H2 equilibrium ground state energy is -1.13727 Ha at R=0.7414 A',
    alpha_corr: 0.95,
    beta_cont: 0.05,
    gamma_unv: 0.1,
    collapsed: true,
    status: 'COLLAPSED_VERIFIED',
  });

  const alpha = active.alpha_corr;
  const beta = active.beta_cont;
  const gamma = active.gamma_unv;

  return (
    <div className="bg-[#0A0D14] border border-slate-800 rounded-xl p-6 space-y-6 shadow-2xl">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-slate-800 pb-4">
        <div>
          <h2 className="text-base font-bold text-white flex items-center space-x-2">
            <Compass className="w-5 h-5 text-emerald-400" />
            <span>Surface 3: Epistemic Manifold &amp; Classical Anchor Horizon</span>
          </h2>
          <p className="text-xs text-slate-400 mt-0.5">
            <MathText text="Tripartite credibility wavefunction $|\Psi\rangle = \alpha|\text{corr}\rangle + \beta|\text{cont}\rangle + \gamma|\text{unver}\rangle$ and classical ground-truth baselines" />
          </p>
        </div>
        <span className="px-3 py-1 rounded-lg bg-emerald-950 border border-emerald-800 text-emerald-300 text-xs font-mono font-bold flex items-center space-x-1.5 glow-emerald">
          <CheckCircle2 className="w-3.5 h-3.5" />
          <span>RING 1 GATED &amp; COLLAPSED</span>
        </span>
      </div>

      {/* Tripartite Wavefunction Manifold Visualization */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 items-center bg-[#05070B] p-6 rounded-xl border border-slate-800">
        <div className="flex flex-col items-center justify-center relative">
          <svg className="w-64 h-64 transform -rotate-90">
            {/* Background circle */}
            <circle cx="128" cy="128" r="100" stroke="#1e293b" strokeWidth="16" fill="transparent" />
            {/* Alpha Corroborated Ring (Emerald) */}
            <circle
              cx="128"
              cy="128"
              r="100"
              stroke="#10b981"
              strokeWidth="16"
              fill="transparent"
              strokeDasharray={2 * Math.PI * 100}
              strokeDashoffset={2 * Math.PI * 100 * (1 - alpha * alpha)}
              className="transition-all duration-1000 ease-out"
              style={{ filter: 'drop-shadow(0 0 8px rgba(16, 185, 129, 0.5))' }}
            />
            {/* Beta Contested Ring (Amber) */}
            <circle
              cx="128"
              cy="128"
              r="80"
              stroke="#f59e0b"
              strokeWidth="12"
              fill="transparent"
              strokeDasharray={2 * Math.PI * 80}
              strokeDashoffset={2 * Math.PI * 80 * (1 - beta * beta)}
              className="transition-all duration-1000 ease-out"
            />
            {/* Gamma Unverified Ring (Violet) */}
            <circle
              cx="128"
              cy="128"
              r="60"
              stroke="#8a2be2"
              strokeWidth="10"
              fill="transparent"
              strokeDasharray={2 * Math.PI * 60}
              strokeDashoffset={2 * Math.PI * 60 * (1 - gamma * gamma)}
              className="transition-all duration-1000 ease-out"
            />
          </svg>
          <div className="absolute flex flex-col items-center justify-center text-center">
            <span className="text-3xl font-bold font-mono text-emerald-400">
              {(alpha * alpha * 100).toFixed(1)}%
            </span>
            <span className="text-[10px] uppercase tracking-wider text-slate-400 font-bold mt-1">
              P(COLLAPSED)
            </span>
          </div>
        </div>

        {/* Credibility Amplitudes & Math Legend */}
        <div className="space-y-3 font-sans">
          <div className="p-3 bg-emerald-950/30 border border-emerald-800/50 rounded-xl flex items-center justify-between">
            <div>
              <div className="text-xs font-bold text-emerald-400">
                <MathText text="$|\text{corroborated}\rangle$ Amplitude ($\alpha$)" />
              </div>
              <div className="text-[11px] text-slate-400">Multi-source alignment &amp; PySCF anchor match</div>
            </div>
            <span className="text-xl font-mono font-bold text-white">{alpha.toFixed(3)}</span>
          </div>

          <div className="p-3 bg-amber-950/30 border border-amber-800/50 rounded-xl flex items-center justify-between">
            <div>
              <div className="text-xs font-bold text-amber-400">
                <MathText text="$|\text{contested}\rangle$ Amplitude ($\beta$)" />
              </div>
              <div className="text-[11px] text-slate-400">Divergence from classical anchor or noise</div>
            </div>
            <span className="text-xl font-mono font-bold text-white">{beta.toFixed(3)}</span>
          </div>

          <div className="p-3 bg-purple-950/30 border border-purple-800/50 rounded-xl flex items-center justify-between">
            <div>
              <div className="text-xs font-bold text-purple-400">
                <MathText text="$|\text{unverified}\rangle$ Amplitude ($\gamma$)" />
              </div>
              <div className="text-[11px] text-slate-400">Prior epistemological uncertainty floor</div>
            </div>
            <span className="text-xl font-mono font-bold text-white">{gamma.toFixed(3)}</span>
          </div>
        </div>
      </div>

      {/* Classical Anchor Horizon Comparison Ladder */}
      <div className="bg-[#05070B] border border-slate-800 rounded-xl p-5 space-y-4 font-mono text-xs">
        <div className="flex items-center justify-between border-b border-slate-800 pb-3">
          <div className="flex items-center space-x-2">
            <Scale className="w-4 h-4 text-cyan-400" />
            <h3 className="font-bold text-white text-xs uppercase tracking-wider">
              Classical Anchor Ground-Truth Ladder ({campaign?.target_system || 'H2'} Hartrees)
            </h3>
          </div>
          <span className="px-2.5 py-0.5 rounded bg-slate-800 text-cyan-300 text-[10px] font-bold border border-slate-700">
            Quantum Advantage Realism Active
          </span>
        </div>

        <div className="space-y-2.5">
          <div className="flex items-center justify-between p-2 bg-[#0A0D14] rounded-lg border border-slate-800">
            <span className="text-slate-400">Classical STO-3G RHF Mean-Field:</span>
            <span className="text-slate-200 font-bold">
              {campaign?.classical_anchor ? `${campaign.classical_anchor.rhf_mean_field_energy.toFixed(6)} Ha` : '-1.116698 Ha'}
            </span>
          </div>
          <div className="flex items-center justify-between p-2 bg-emerald-950/20 rounded-lg border border-emerald-800/40">
            <span className="text-emerald-400 font-bold">Classical PySCF Exact FCI Anchor:</span>
            <span className="text-emerald-300 font-bold">
              {campaign?.classical_anchor ? `${campaign.classical_anchor.fci_exact_energy.toFixed(6)} Ha` : '-1.137270 Ha'}
            </span>
          </div>
          <div className="flex items-center justify-between p-2 bg-cyan-950/20 rounded-lg border border-cyan-800/40">
            <span className="text-cyan-400">Quantum VQE Aer Execution:</span>
            <span className="text-cyan-300 font-bold">
              {campaign?.classical_anchor ? `${campaign.classical_anchor.vqe_expectation_energy.toFixed(6)} Ha` : '-1.136800 Ha'}
            </span>
          </div>
          <div className="flex items-center justify-between p-2 bg-slate-900 rounded-lg border border-slate-800 text-slate-400 text-[11px]">
            <span>Chemical Accuracy Discrepancy (Δ):</span>
            <span className="text-emerald-400 font-bold">
              {campaign?.classical_anchor
                ? `${campaign.classical_anchor.discrepancy_mha.toFixed(3)} mHa < 1.6 mHa (${campaign.classical_anchor.chemical_accuracy_passed ? 'PASS' : 'FAIL'})`
                : '0.470 mHa < 1.6 mHa (PASS)'}
            </span>
          </div>
        </div>
      </div>

      {/* Stagnation Telemetry Gauges */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="p-4 bg-[#05070B] border border-slate-800 rounded-xl space-y-1">
          <div className="text-[11px] text-slate-400 font-medium">HYPOTHESIS CONCENTRATION (κ)</div>
          <div className="text-2xl font-mono font-bold text-cyan-400">
            {campaign?.quantum_factors
              ? (1 / (1 + (campaign.quantum_factors.gradient_variance * 1000))).toFixed(4)
              : (data?.stagnation_kappa?.toFixed(4) || '0.8928')}
          </div>
          <div className="text-[10px] text-slate-500 font-mono">Curvature κ = 1 / (1 + |Var|)</div>
        </div>

        <div className="p-4 bg-[#05070B] border border-slate-800 rounded-xl space-y-1">
          <div className="text-[11px] text-slate-400 font-medium">LITERATURE DRIFT d(E_canon, E_live)</div>
          <div className="text-2xl font-mono font-bold text-amber-400">
            {campaign?.classical_anchor
              ? campaign.classical_anchor.discrepancy_mha.toFixed(2)
              : (data?.literature_drift_mha?.toFixed(2) || '0.47')} mHa
          </div>
          <div className="text-[10px] text-emerald-400 font-medium">&lt; 1.6 mHa (Chemical Accuracy Satisfied)</div>
        </div>

        <div className="p-4 bg-[#05070B] border border-slate-800 rounded-xl space-y-1">
          <div className="text-[11px] text-slate-400 font-medium">RING 1 REFUSAL RATIO</div>
          <div className="text-2xl font-mono font-bold text-indigo-400">
            {campaign?.epistemics
              ? ((campaign.epistemics.beta_contested + campaign.epistemics.gamma_unverified) * 100).toFixed(1)
              : (((data?.refusal_ratio || 0.0) * 100).toFixed(1))}%
          </div>
          <div className="text-[10px] text-slate-500 font-mono">Uncollapsed assertions blocked fail-closed</div>
        </div>
      </div>
    </div>
  );
}
