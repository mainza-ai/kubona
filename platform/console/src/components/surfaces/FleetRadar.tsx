'use client';

import React, { useState, useEffect, useRef } from 'react';
import {
  Radio,
  ShieldCheck,
  Cpu,
  Lock,
  Globe,
  Star,
  ChevronDown,
  ChevronUp,
  MapPin,
} from 'lucide-react';
import { CampaignState, BackendInfo, RadarEndpoint } from '@/types';
import { fetchFleet, fetchRadar } from '@/lib/api';

interface FleetRadarProps {
  campaign?: CampaignState | null;
}

export function FleetRadar({ campaign }: FleetRadarProps) {
  const [backends, setBackends] = useState<BackendInfo[]>([]);
  const [radarEndpoints, setRadarEndpoints] = useState<RadarEndpoint[]>([]);
  const [sovereignOnly, setSovereignOnly] = useState(false);
  const [expandedBackend, setExpandedBackend] = useState<string | null>(null);
  const [refused, setRefused] = useState<string | null>(null);
  const backoffRef = useRef(1000);

  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const pollRadar = async () => {
      // Visibility-gated: no polling when the tab is hidden.
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
        if (active) timer = setTimeout(pollRadar, 2000);
        return;
      }
      try {
        const data = await fetchRadar();
        if (!active) return;
        if ((data as { refused?: boolean })?.refused) {
          setRefused((data as { refuse_reason?: string })?.refuse_reason || 'radar refused');
          setRadarEndpoints([]);
          backoffRef.current = Math.min(backoffRef.current * 1.5, 15000);
        } else if (data?.radar_endpoints) {
          setRadarEndpoints(data.radar_endpoints as RadarEndpoint[]);
          setRefused(null);
          backoffRef.current = 1000;
        }
      } catch (err) {
        if (!active) return;
        setRefused(err instanceof Error ? err.message : String(err));
        backoffRef.current = Math.min(backoffRef.current * 1.5, 15000);
      }
      if (active) timer = setTimeout(pollRadar, backoffRef.current);
    };
    fetchFleet()
      .then((data) => {
        if (!active) return;
        if ((data as { refused?: boolean })?.refused) {
          setRefused((data as { refuse_reason?: string })?.refuse_reason || 'fleet refused');
          setBackends([]);
        } else {
          setBackends((data.backends as BackendInfo[]) || []);
        }
      })
      .catch((err) => {
        if (active) setRefused(err instanceof Error ? err.message : String(err));
      });

    pollRadar();
    return () => {
      active = false;
      if (timer) clearTimeout(timer);
    };
  }, []);

  const filteredBackends = sovereignOnly
    ? backends.filter((b) => b.jurisdiction === 'LOCAL_SOVEREIGN' || (b.name || '').toLowerCase().includes('local'))
    : backends;
  // EAR99 banner only when a backend flag actually carries it — never static.
  const showEar99 = backends.some((b) => ((b.itar_ear || '') as string).toUpperCase().includes('EAR99'));

  const tiers = [
    { tier: 0, title: 'Tier 0 · Classical Physics Ground Truth', desc: 'PySCF FCI / RHF ab-initio exact anchor' },
    { tier: 1, title: 'Tier 1 · Sovereign Local Emulation', desc: 'Qiskit Aer GPU/MPS, ZNE, Wire-Cutting' },
    { tier: 2, title: 'Tier 2 · Managed Cloud Simulators', desc: 'AWS Braket SV1/TN1, CUDA-Q accelerated' },
    { tier: 3, title: 'Tier 3 · Physical QPUs', desc: 'IBM Heron (156q), Quantinuum H1 (20q), Rigetti' },
  ];

  return (
    <div className="relative rounded-3xl overflow-hidden bg-gradient-to-b from-[#090C13] via-[#0B0E17] to-[#06080D] border border-white/[0.08] shadow-2xl p-7 space-y-6 select-none">
      {/* Serene Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-white/[0.08] pb-5">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-2xl bg-cyan-500/10 border border-cyan-500/20 backdrop-blur-xl flex items-center justify-center shadow-inner">
            <Radio className="w-5 h-5 text-cyan-400" />
          </div>
          <div>
            <h2 className="text-base font-semibold text-white tracking-tight flex items-center space-x-2">
              <span>Ambient Hardware Radar</span>
              <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
            </h2>
            <p className="text-xs text-slate-400 mt-0.5">
              4-Tier classical-quantum continuum with real-time coherence calibration and sovereign routing
            </p>
          </div>
        </div>

        {/* Local-only list filter (display filter — enforcement lives in routing/admission) */}
        <div className="flex items-center space-x-3">
          <button
            onClick={() => setSovereignOnly(!sovereignOnly)}
            title="Filters this list to local backends; does not change enforcement"
            className={`px-3.5 py-1.5 rounded-xl text-xs font-semibold flex items-center space-x-2 border transition-all shadow-sm active:scale-95 ${
              sovereignOnly
                ? 'bg-emerald-950/70 border-emerald-500/40 text-emerald-300 ring-1 ring-emerald-500/20'
                : 'bg-white/[0.04] hover:bg-white/[0.08] border-white/10 text-slate-300'
            }`}
          >
            {sovereignOnly ? (
              <>
                <Lock className="w-3.5 h-3.5 text-emerald-400" />
                <span>Local backends only (filter)</span>
              </>
            ) : (
              <>
                <Globe className="w-3.5 h-3.5 text-slate-400" />
                <span>Global Fleet Available</span>
              </>
            )}
          </button>
        </div>
      </div>

      {/* Continuum Radar Strip */}
      <div className="p-4 rounded-2xl bg-black/40 border border-white/[0.06] backdrop-blur-xl space-y-3">
        <div className="flex items-center justify-between text-xs">
          <span className="font-semibold text-slate-300 uppercase tracking-wider font-mono text-[11px]">
            Quantum Execution Continuum
          </span>
          {showEar99 && (
            <span className="text-slate-400 font-mono text-[10px] flex items-center space-x-1.5">
              <ShieldCheck className="w-3.5 h-3.5 text-emerald-400" />
              <span>EAR99 Export-Controlled Cloud Filtering Active</span>
            </span>
          )}
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-2.5">
          {tiers.map((t) => (
            <div
              key={t.tier}
              className="p-3 rounded-xl bg-white/[0.02] border border-white/[0.05] space-y-1 hover:border-cyan-500/30 transition-all"
            >
              <div className="text-xs font-bold text-white flex items-center space-x-1.5">
                <span className="w-1.5 h-1.5 rounded-full bg-cyan-400" />
                <span>Tier {t.tier}</span>
              </div>
              <p className="text-[11px] text-slate-400 leading-tight font-sans">
                {t.desc}
              </p>
            </div>
          ))}
        </div>
      </div>

      {/* Real-Time Host Daemon Telemetry (Ports 8080 & 50051) */}
      <div className="space-y-3">
        <div className="flex items-center justify-between text-xs font-semibold text-slate-300 font-mono uppercase tracking-wider">
          <span>Host Daemon Telemetry &amp; Gateway Links</span>
          <span className="text-[10px] text-cyan-400 font-mono flex items-center space-x-1.5">
            <span className="w-1.5 h-1.5 rounded-full bg-cyan-400 animate-ping" />
            <span>1000ms Live Telemetry Radar</span>
          </span>
        </div>

        {refused && (
          <div className="p-2.5 rounded-xl bg-rose-950/60 border border-rose-500/40 text-rose-200 text-[11px]">
            Radar refused — {refused.slice(0, 200)}. Nothing discovered, nothing assumed.
          </div>
        )}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3.5">
          {/* Port 8080 Go Core Orchestrator Gateway */}
          {(() => {
            // Outage defaults unknown (not LISTENING): absent endpoint renders
            // UNKNOWN, never a confident LISTENING.
            const ep8080 = radarEndpoints.find((e) => e.port === 8080);
            const state: 'on' | 'off' | 'unknown' = ep8080 == null ? 'unknown' : ep8080.listening ? 'on' : 'off';
            return (
              <div
                data-testid="radar-endpoint-8080"
                className="p-4 rounded-2xl bg-black/40 border border-white/[0.08] backdrop-blur-xl flex items-center justify-between transition-all"
              >
                <div className="space-y-1">
                  <div className="flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-cyan-400" />
                    <span className="text-xs font-mono font-bold text-white">Port 8080</span>
                    <span className="text-[10px] font-mono text-cyan-300 bg-cyan-950/80 px-2 py-0.5 rounded border border-cyan-800/40">
                      HTTP / REST
                    </span>
                  </div>
                  <p className="text-xs text-slate-300 font-sans">
                    Kubona Go Core Gateway (Daemon: kubonad)
                  </p>
                </div>
                <div
                  data-testid="status-indicator-8080"
                  className={`px-3 py-1 rounded-full text-[11px] font-mono font-semibold flex items-center space-x-1.5 border transition-all ${
                    state === 'on'
                      ? 'bg-emerald-950/70 text-emerald-300 border-emerald-700/60'
                      : state === 'unknown'
                        ? 'bg-white/[0.06] text-slate-300 border-white/20'
                        : 'bg-rose-950/70 text-rose-300 border-rose-700/60'
                  }`}
                >
                  <span
                    className={`w-2 h-2 rounded-full ${
                      state === 'on' ? 'bg-emerald-400 animate-pulse' : state === 'unknown' ? 'bg-slate-400' : 'bg-rose-500'
                    }`}
                  />
                  <span>{state === 'on' ? 'LISTENING / SOVEREIGN' : state === 'unknown' ? 'UNKNOWN — no endpoint' : 'OFFLINE / DISCONNECTED'}</span>
                </div>
              </div>
            );
          })()}

          {/* Port 50051 Python Quantum Bridge */}
          {(() => {
            const ep50051 = radarEndpoints.find((e) => e.port === 50051);
            const state: 'on' | 'off' | 'unknown' = ep50051 == null ? 'unknown' : ep50051.listening ? 'on' : 'off';
            return (
              <div
                data-testid="radar-endpoint-50051"
                className="p-4 rounded-2xl bg-black/40 border border-white/[0.08] backdrop-blur-xl flex items-center justify-between transition-all"
              >
                <div className="space-y-1">
                  <div className="flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-full bg-indigo-400" />
                    <span className="text-xs font-mono font-bold text-white">Port 50051</span>
                    <span className="text-[10px] font-mono text-indigo-300 bg-indigo-950/80 px-2 py-0.5 rounded border border-indigo-800/40">
                      gRPC
                    </span>
                  </div>
                  <p className="text-xs text-slate-300 font-sans">
                    Kubona Python Quantum Bridge (Daemon: server.py)
                  </p>
                </div>
                <div
                  data-testid="status-indicator-50051"
                  className={`px-3 py-1 rounded-full text-[11px] font-mono font-semibold flex items-center space-x-1.5 border transition-all ${
                    state === 'on'
                      ? 'bg-emerald-950/70 text-emerald-300 border-emerald-700/60'
                      : state === 'unknown'
                        ? 'bg-white/[0.06] text-slate-300 border-white/20'
                        : 'bg-rose-950/70 text-rose-300 border-rose-700/60'
                  }`}
                >
                  <span
                    className={`w-2 h-2 rounded-full ${
                      state === 'on' ? 'bg-emerald-400 animate-pulse' : state === 'unknown' ? 'bg-slate-400' : 'bg-rose-500'
                    }`}
                  />
                  <span>{state === 'on' ? 'LISTENING / SOVEREIGN' : state === 'unknown' ? 'UNKNOWN — no endpoint' : 'OFFLINE / DISCONNECTED'}</span>
                </div>
              </div>
            );
          })()}
        </div>
      </div>

      {/* Backend Devices: Clean Minimal Cards with Progressive Disclosure */}
      <div className="space-y-3">
        <div className="text-xs font-semibold text-slate-300 font-mono uppercase tracking-wider">
          Available Devices &amp; Simulators ({filteredBackends.length})
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {filteredBackends.map((b) => {
            const isRecommended =
              campaign?.quantum_factors?.recommended_backend &&
              ((b.name.toLowerCase().includes('aer') && campaign.quantum_factors.recommended_backend.toLowerCase().includes('aer')) ||
                (b.name.toLowerCase().includes('heron') && campaign.quantum_factors.recommended_backend.toLowerCase().includes('heron')));

            const isExpanded = expandedBackend === b.name;
            const statusUp = (b.status || '').toUpperCase();
            const isOnline = statusUp === 'ONLINE' || statusUp === 'READY';

            return (
              <div
                key={b.name}
                className={`p-5 rounded-2xl bg-black/40 border transition-all backdrop-blur-xl space-y-3.5 ${
                  isRecommended
                    ? 'border-cyan-500/50 ring-1 ring-cyan-500/30 shadow-lg shadow-cyan-950/20'
                    : 'border-white/[0.07] hover:border-white/15'
                }`}
              >
                {/* Device Title & Status Pill */}
                <div className="flex items-center justify-between">
                  <div className="flex items-center space-x-2.5">
                    <Cpu className={`w-4 h-4 ${isRecommended ? 'text-cyan-300' : 'text-slate-400'}`} />
                    <span className="font-bold text-sm text-white font-mono tracking-tight">{b.name}</span>
                    {isRecommended && (
                      <span className="px-2 py-0.5 rounded-md bg-cyan-950/80 text-cyan-300 border border-cyan-800 text-[9px] font-mono font-bold flex items-center space-x-1">
                        <Star className="w-2.5 h-2.5 fill-cyan-400 text-cyan-400" />
                        <span>RECOMMENDED</span>
                      </span>
                    )}
                  </div>

                  <span
                    className={`px-2.5 py-0.5 rounded-full text-[10px] font-mono font-semibold flex items-center space-x-1.5 border ${
                      isOnline
                        ? 'bg-emerald-950/60 text-emerald-300 border-emerald-800/60'
                        : 'bg-amber-950/60 text-amber-300 border-amber-800/60'
                    }`}
                  >
                    <span className={`w-1.5 h-1.5 rounded-full ${isOnline ? 'bg-emerald-400 animate-pulse' : 'bg-amber-400'}`} />
                    <span>{b.status}</span>
                  </span>
                </div>

                <div className="text-xs text-slate-300 font-sans">{b.type}</div>

                {/* Depth 1: Essential Metrics */}
                <div className="grid grid-cols-3 gap-2 text-xs font-mono bg-white/[0.03] p-3 rounded-xl border border-white/10">
                  <div>
                    <span className="text-slate-300 block text-[11px] font-medium">Capacity</span>
                    <strong className="text-white text-xs font-bold">{b.qubits} Qubits</strong>
                  </div>
                  <div>
                    <span className="text-slate-300 block text-[11px] font-medium">Coherence T₁</span>
                    <strong className="text-cyan-300 text-xs font-bold">{b.t1_us} µs</strong>
                  </div>
                  <div>
                    <span className="text-slate-300 block text-[11px] font-medium">Queue</span>
                    <strong className="text-amber-300 text-xs font-bold">{b.queue_depth} jobs</strong>
                  </div>
                </div>

                {/* Progressive Disclosure Toggle */}
                <button
                  onClick={() => setExpandedBackend(isExpanded ? null : b.name)}
                  className="w-full flex items-center justify-between text-xs text-slate-300 hover:text-white font-medium transition-colors pt-2 border-t border-white/[0.08]"
                >
                  <span className="flex items-center space-x-1.5">
                    <MapPin className="w-3.5 h-3.5 text-slate-300" />
                    <span>{b.jurisdiction} ({b.itar_ear})</span>
                  </span>
                  <span className="flex items-center space-x-1 font-mono text-cyan-300">
                    <span>{isExpanded ? 'Hide Specs' : 'Deep Physics & Pricing'}</span>
                    {isExpanded ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
                  </span>
                </button>

                {/* Depth 2 & 3: Physics & Cost Specs */}
                {isExpanded && (
                  <div className="p-3.5 rounded-xl bg-black/70 border border-white/10 space-y-2 text-xs font-mono animate-fadeIn">
                    <div className="flex justify-between text-slate-300 font-medium">
                      <span>Cost per Shot:</span>
                      <strong className="text-emerald-300">${typeof b.cost_per_shot === 'number' && Number.isFinite(b.cost_per_shot) ? b.cost_per_shot.toFixed(4) : '—'}</strong>
                    </div>
                    {b.avg_t2_us !== undefined && b.avg_t2_us > 0 && (
                      <div className="flex justify-between text-slate-300 font-medium">
                        <span>Coherence T₂ (Dephasing):</span>
                        <strong className="text-cyan-300">{b.avg_t2_us} µs</strong>
                      </div>
                    )}
                    <div className="flex justify-between text-slate-300 font-medium">
                      <span>Jurisdiction Policy:</span>
                      <span className="text-white font-semibold">{b.jurisdiction} · {b.itar_ear}</span>
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
