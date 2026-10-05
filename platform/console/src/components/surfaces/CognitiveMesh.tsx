'use client';

import React, { useState, useEffect } from 'react';
import {
  Network,
  RefreshCw,
  Lock,
  Globe,
  Terminal,
  Users,
} from 'lucide-react';
import { CampaignState } from '@/types';
import { fetchRadar, fetchLLMSettings, saveLLMSettings } from '@/lib/api';

interface RadarEndpoint {
  port: number;
  listening: boolean;
  service: string;
  sovereign: boolean;
  is_local_ip: boolean;
}

interface CognitiveMeshProps {
  campaign?: CampaignState | null;
}

export function CognitiveMesh({ campaign }: CognitiveMeshProps) {
  const [endpoints, setEndpoints] = useState<RadarEndpoint[]>([]);
  const [airGapped, setAirGapped] = useState(true);
  const [loading, setLoading] = useState(false);

  const scanRadar = async () => {
    setLoading(true);
    try {
      const data = await fetchRadar();
      setEndpoints((data.radar_endpoints as RadarEndpoint[]) || []);
    } catch (err) {
      console.error('Failed to scan host radar:', err);
    } finally {
      setLoading(false);
    }
  };

  const toggleAirGap = async () => {
    const nextVal = !airGapped;
    setAirGapped(nextVal);
    try {
      await saveLLMSettings({ air_gapped: nextVal });
    } catch (err) {
      console.error('Failed to save air gap state:', err);
    }
  };

  useEffect(() => {
    let mounted = true;
    fetchRadar()
      .then((data) => {
        if (mounted && data?.radar_endpoints) {
          setEndpoints(data.radar_endpoints as RadarEndpoint[]);
        }
      })
      .catch((err) => console.error('Failed to scan host radar:', err));

    fetchLLMSettings()
      .then((data) => {
        if (mounted && data?.air_gapped !== undefined) {
          setAirGapped(Boolean(data.air_gapped));
        }
      })
      .catch((err) => console.error('Failed to load LLM settings:', err));

    return () => {
      mounted = false;
    };
  }, []);

  const roles = [
    { role: 'Epistemic Scribe', model: 'oMLX: Qwen3.8-27B-Uncensored-OrcaRouter (Port 8787)', t: 'T=0.0', desc: `Synthesizes verified theorems for ${campaign?.target_system || 'Target'} into citable manuscripts` },
    { role: 'Ansatz Scout', model: 'oMLX: Qwen3.8-27B-Uncensored-OrcaRouter (Port 8787)', t: 'T=0.2', desc: 'Explores hardware-efficient parameterizations without barren plateaus' },
    { role: 'Disconfirmation Officer', model: 'oMLX: Qwen3.8-27B-Uncensored-OrcaRouter (Port 8787)', t: 'T=0.0', desc: 'Challenges findings with classical ground truth & noise simulations' },
    { role: 'Device Auditor', model: 'oMLX: Qwen3.8-27B-Uncensored-OrcaRouter (Port 8787)', t: 'T=0.0', desc: 'Calibrates Clifford RB fidelities & coherence windows' },
    { role: 'Human Principal', model: 'Browser WebCrypto Client-Side Ed25519 Keyring', t: 'Manual AP2', desc: 'Cryptographically signs hardware dispatches & budget approvals' },
  ];

  return (
    <div className="relative rounded-3xl overflow-hidden bg-gradient-to-b from-[#090C13] via-[#0B0E17] to-[#06080D] border border-white/[0.08] shadow-2xl p-7 space-y-6 select-none">
      {/* Serene Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-white/[0.08] pb-5">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-2xl bg-indigo-500/10 border border-indigo-500/20 backdrop-blur-xl flex items-center justify-center shadow-inner">
            <Network className="w-5 h-5 text-indigo-400" />
          </div>
          <div>
            <h2 className="text-base font-semibold text-white tracking-tight flex items-center space-x-2">
              <span>Sovereign Cognitive Mesh</span>
              <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
            </h2>
            <p className="text-xs text-slate-400 mt-0.5">
              Zero-config local host port discovery, sovereign air-gap isolation, and multi-agent tribunal matrix
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-3">
          <button
            onClick={scanRadar}
            disabled={loading}
            className="px-3 py-1.5 bg-white/[0.04] hover:bg-white/[0.08] text-slate-200 text-xs font-semibold rounded-xl flex items-center space-x-1.5 border border-white/10 transition-colors shadow-sm"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            <span>Scan Ports</span>
          </button>

          {/* Sovereign Air-Gap Toggle */}
          <button
            onClick={toggleAirGap}
            className={`px-3.5 py-1.5 rounded-xl text-xs font-semibold flex items-center space-x-2 border transition-all shadow-sm active:scale-95 ${
              airGapped
                ? 'bg-emerald-950/70 border-emerald-500/40 text-emerald-300 ring-1 ring-emerald-500/20'
                : 'bg-white/[0.04] hover:bg-white/[0.08] border-white/10 text-slate-300'
            }`}
          >
            {airGapped ? (
              <>
                <Lock className="w-3.5 h-3.5 text-emerald-400" />
                <span>Air-Gap: ENGAGED</span>
              </>
            ) : (
              <>
                <Globe className="w-3.5 h-3.5 text-slate-400" />
                <span>Air-Gap: OFF</span>
              </>
            )}
          </button>
        </div>
      </div>

      {/* Host Port Radar Discovery */}
      <div className="p-5 rounded-2xl bg-black/40 border border-white/[0.06] backdrop-blur-xl space-y-3">
        <div className="flex items-center justify-between text-xs">
          <span className="font-semibold text-slate-300 uppercase tracking-wider font-mono text-[11px] flex items-center space-x-2">
            <Terminal className="w-4 h-4 text-cyan-400" />
            <span>Local Host Port Radar (Zero-Config Discovery)</span>
          </span>
          <span className="text-slate-400 font-mono text-[10px]">
            Target: 127.0.0.1 (Loopback Only)
          </span>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {endpoints.map((ep) => (
            <div
              key={ep.port}
              className="p-3.5 rounded-xl bg-white/[0.02] border border-white/[0.05] flex items-center justify-between text-xs font-mono hover:border-cyan-500/30 transition-all"
            >
              <div>
                <div className="font-semibold text-white">{ep.service}</div>
                <div className="text-slate-300 text-xs font-mono">127.0.0.1:{ep.port}</div>
              </div>
              <span
                className={`px-2.5 py-0.5 rounded-full text-[10px] font-semibold flex items-center space-x-1.5 border ${
                  ep.listening
                    ? 'bg-emerald-950/60 text-emerald-300 border-emerald-800/60'
                    : 'bg-white/[0.04] text-slate-300 border-white/10'
                }`}
              >
                <span className={`w-1.5 h-1.5 rounded-full ${ep.listening ? 'bg-emerald-400 animate-pulse' : 'bg-slate-400'}`} />
                <span>{ep.listening ? 'ONLINE' : 'IDLE'}</span>
              </span>
            </div>
          ))}
        </div>
      </div>

      {/* Research Salon Specialist Persona Matrix */}
      <div className="p-5 rounded-2xl bg-black/40 border border-white/[0.06] backdrop-blur-xl space-y-4">
        <div className="flex items-center justify-between text-xs">
          <span className="font-semibold text-slate-300 uppercase tracking-wider font-mono text-[11px] flex items-center space-x-2">
            <Users className="w-4 h-4 text-indigo-400" />
            <span>Epistemic Tribunal Agent Allocations</span>
          </span>
          <span className="text-slate-300 font-mono text-[10px]">
            Decentralized Peer Consensus
          </span>
        </div>

        <div className="grid grid-cols-1 gap-2.5">
          {roles.map((r) => (
            <div
              key={r.role}
              className="p-3.5 rounded-xl bg-white/[0.02] border border-white/[0.05] flex flex-wrap items-center justify-between gap-3 text-xs hover:border-indigo-500/30 transition-all"
            >
              <div className="space-y-0.5">
                <div className="flex items-center space-x-2">
                  <span className="font-bold text-white tracking-tight">{r.role}</span>
                  <span className="px-1.5 py-0.2 rounded bg-indigo-950/80 text-indigo-300 border border-indigo-800 text-[9px] font-mono">
                    {r.t}
                  </span>
                </div>
                <p className="text-xs text-slate-300 font-sans">{r.desc}</p>
              </div>

              <span className="text-cyan-300 font-mono text-[11px] bg-black/40 px-3 py-1 rounded-lg border border-white/[0.06]">
                {r.model}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
