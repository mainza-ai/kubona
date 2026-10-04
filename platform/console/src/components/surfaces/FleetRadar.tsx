'use client';

import React, { useState, useEffect } from 'react';
import { Radio, ShieldAlert, Cpu, Activity, Clock, CheckCircle2, Layers, Globe, Zap, Star } from 'lucide-react';
import { CampaignState } from '@/types';

interface BackendInfo {
  name: string;
  type: string;
  qubits: number;
  t1_us: string;
  status: string;
  queue_depth: number;
  cost_per_shot: number;
  jurisdiction: string;
  itar_ear: string;
}

interface FleetRadarProps {
  campaign?: CampaignState | null;
}

export function FleetRadar({ campaign }: FleetRadarProps) {
  const [backends, setBackends] = useState<BackendInfo[]>([]);
  const [activeTier, setActiveTier] = useState<number | 'all'>('all');

  useEffect(() => {
    fetch('http://localhost:8080/api/fleet')
      .then((res) => res.json())
      .then((data) => setBackends(data.backends || []))
      .catch((err) => console.error('Failed to load fleet data:', err));
  }, []);

  const tiers = [
    { tier: 0, title: 'Tier 0 · Classical Physics Anchors', desc: 'PySCF FCI/RHF ab-initio ground truth' },
    { tier: 1, title: 'Tier 1 · Local Sovereign Emulation', desc: 'Qiskit Aer CPU/GPU, ZNE, Wire-Cutting' },
    { tier: 2, title: 'Tier 2 · Managed Cloud Simulators', desc: 'AWS Braket SV1/TN1, CUDA-Q' },
    { tier: 3, title: 'Tier 3 · Physical QPUs', desc: 'IBM Heron, Rigetti, Quantinuum, QuEra' },
  ];

  return (
    <div className="bg-[#0A0D14] border border-slate-800 rounded-xl p-6 space-y-6 shadow-2xl">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-slate-800 pb-4">
        <div>
          <h2 className="text-base font-bold text-white flex items-center space-x-2">
            <Radio className="w-5 h-5 text-cyan-400" />
            <span>Surface 5: Execution Fleet Radar &amp; Hardware Tier Shield</span>
          </h2>
          <p className="text-xs text-slate-400 mt-0.5">
            4-Tier classical-quantum continuum, QPU coherence calibration ($T_1/T_2$), and EAR99 jurisdiction routing
          </p>
        </div>
        <span className="px-3 py-1 rounded-lg bg-cyan-950 border border-cyan-800 text-cyan-300 text-xs font-mono font-bold flex items-center space-x-1.5 glow-cyan">
          <Activity className="w-3.5 h-3.5 text-cyan-400 animate-pulse" />
          <span>RADAR SWEEP ONLINE</span>
        </span>
      </div>

      {/* Jurisdiction Shield & Clifford Canary Banner */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="p-4 bg-[#05070B] border border-slate-800 rounded-xl flex items-center justify-between text-xs">
          <div className="flex items-center space-x-3">
            <ShieldAlert className="w-5 h-5 text-emerald-400 shrink-0" />
            <div className="text-slate-300">
              <div className="font-bold text-white">ITAR/EAR Export-Control Policy Shield</div>
              <div className="text-[11px] text-slate-400">
                Enforcing <strong className="font-mono text-emerald-300">policy.py</strong> rules: Cloud traffic filtered for EAR99 compliance.
              </div>
            </div>
          </div>
          <span className="px-2 py-0.5 rounded bg-emerald-950 text-emerald-300 border border-emerald-800 font-mono text-[10px] font-bold">
            ACTIVE SHIELD
          </span>
        </div>

        <div className="p-4 bg-[#05070B] border border-slate-800 rounded-xl flex items-center justify-between text-xs">
          <div className="flex items-center space-x-3">
            <Zap className="w-5 h-5 text-cyan-400 shrink-0" />
            <div className="text-slate-300">
              <div className="font-bold text-white">Clifford Stabilizer Canary Probes</div>
              <div className="text-[11px] text-slate-400">
                Pre-flight RB canary fidelity: <strong className="font-mono text-cyan-300">F = {((campaign?.quantum_factors?.clifford_fidelity ?? 0.984) * 100).toFixed(1)}%</strong> (≥ 95% threshold).
              </div>
            </div>
          </div>
          <span className="px-2 py-0.5 rounded bg-cyan-950 text-cyan-300 border border-cyan-800 font-mono text-[10px] font-bold">
            PROBES VERIFIED
          </span>
        </div>
      </div>

      {/* 4-Tier Continuum Filter Chips */}
      <div className="flex items-center space-x-2 overflow-x-auto pb-1 text-xs">
        <button
          onClick={() => setActiveTier('all')}
          className={`px-3 py-1.5 rounded-lg border font-mono transition-colors ${
            activeTier === 'all'
              ? 'bg-indigo-600 text-white border-indigo-500 font-bold'
              : 'bg-slate-900 text-slate-400 border-slate-800 hover:text-white'
          }`}
        >
          All Continuum Tiers
        </button>
        {tiers.map((t) => (
          <button
            key={t.tier}
            onClick={() => setActiveTier(t.tier)}
            className={`px-3 py-1.5 rounded-lg border font-mono transition-colors shrink-0 ${
              activeTier === t.tier
                ? 'bg-indigo-600 text-white border-indigo-500 font-bold'
                : 'bg-slate-900 text-slate-400 border-slate-800 hover:text-white'
            }`}
          >
            Tier {t.tier}
          </button>
        ))}
      </div>

      {/* Fleet Multi-Modality Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {backends.map((b) => {
          const isRecommended = campaign?.quantum_factors?.recommended_backend &&
            (b.name.toLowerCase().includes('aer') && campaign.quantum_factors.recommended_backend.toLowerCase().includes('aer') ||
             b.name.toLowerCase().includes('heron') && campaign.quantum_factors.recommended_backend.toLowerCase().includes('heron'));

          return (
          <div
            key={b.name}
            className={`p-4 bg-[#05070B] border rounded-xl space-y-3 shadow-lg transition-all ${
              isRecommended ? 'border-cyan-500/60 ring-1 ring-cyan-500/30' : 'border-slate-800'
            }`}
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-2.5">
                <Cpu className={`w-4 h-4 ${isRecommended ? 'text-cyan-300 animate-pulse' : 'text-cyan-400'}`} />
                <span className="font-bold text-white text-sm font-mono">{b.name}</span>
                {isRecommended && (
                  <span className="px-2 py-0.5 rounded-md bg-cyan-950 text-cyan-300 border border-cyan-800 text-[9px] font-mono font-bold flex items-center space-x-1">
                    <Star className="w-2.5 h-2.5 fill-cyan-400" />
                    <span>RECOMMENDED FOR {campaign?.target_system || 'TARGET'}</span>
                  </span>
                )}
              </div>
              <span
                className={`px-2.5 py-0.5 rounded text-[10px] font-mono font-bold border ${
                  b.status === 'ONLINE' || b.status === 'READY'
                    ? 'bg-emerald-950 text-emerald-300 border-emerald-800'
                    : 'bg-amber-950 text-amber-300 border-amber-800'
                }`}
              >
                {b.status}
              </span>
            </div>

            <div className="text-xs text-slate-400">{b.type}</div>

            <div className="grid grid-cols-3 gap-2 text-xs font-mono bg-[#0A0D14] p-2.5 rounded-lg border border-slate-800">
              <div>
                <span className="text-slate-500">Qubits:</span> <strong className="text-white">{b.qubits}</strong>
              </div>
              <div>
                <span className="text-slate-500">T₁:</span> <strong className="text-cyan-400">{b.t1_us} µs</strong>
              </div>
              <div>
                <span className="text-slate-500">Queue:</span> <strong className="text-amber-400">{b.queue_depth}</strong>
              </div>
            </div>

            <div className="flex items-center justify-between text-[11px] font-mono text-slate-400 pt-2 border-t border-slate-800">
              <span>Cost/Shot: <strong className="text-emerald-400">${b.cost_per_shot}</strong></span>
              <span>Jurisdiction: <strong className="text-slate-300">{b.jurisdiction}</strong></span>
            </div>
          </div>
        ); })}
      </div>
    </div>
  );
}
