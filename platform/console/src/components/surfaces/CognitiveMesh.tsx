'use client';

import React, { useState, useEffect } from 'react';
import { Network, ShieldCheck, Power, RefreshCw, Cpu, CheckCircle, Lock, Globe, Terminal, Sliders } from 'lucide-react';

interface RadarEndpoint {
  port: number;
  listening: boolean;
  service: string;
  sovereign: boolean;
  is_local_ip: boolean;
}

export function CognitiveMesh() {
  const [endpoints, setEndpoints] = useState<RadarEndpoint[]>([]);
  const [airGapped, setAirGapped] = useState(true);
  const [loading, setLoading] = useState(false);

  const scanRadar = async () => {
    setLoading(true);
    try {
      const res = await fetch('http://localhost:8080/api/radar');
      const data = await res.json();
      setEndpoints(data.radar_endpoints || []);
    } catch (err) {
      console.error('Failed to scan host radar:', err);
    } finally {
      setLoading(false);
    }
  };

  const loadSettings = async () => {
    try {
      const res = await fetch('http://localhost:8080/api/llm/settings');
      if (res.ok) {
        const data = await res.json();
        if (data.air_gapped !== undefined) {
          setAirGapped(data.air_gapped);
        }
      }
    } catch (err) {
      console.error('Failed to load LLM settings:', err);
    }
  };

  const toggleAirGap = async () => {
    const nextVal = !airGapped;
    setAirGapped(nextVal);
    try {
      await fetch('http://localhost:8080/api/llm/settings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ air_gapped: nextVal }),
      });
    } catch (err) {
      console.error('Failed to save air gap state:', err);
    }
  };

  useEffect(() => {
    scanRadar();
    loadSettings();
  }, []);

  const roles = [
    { role: 'Epistemic Scribe', model: 'oMLX: Qwen3.8-27B-Uncensored-OrcaRouter (Port 8787)', t: 'T=0.0' },
    { role: 'Ansatz Scout', model: 'oMLX: Qwen3.8-27B-Uncensored-OrcaRouter (Port 8787)', t: 'T=0.2' },
    { role: 'Disconfirmation Officer', model: 'oMLX: Qwen3.8-27B-Uncensored-OrcaRouter (Port 8787)', t: 'T=0.0' },
    { role: 'Device Auditor', model: 'oMLX: Qwen3.8-27B-Uncensored-OrcaRouter (Port 8787)', t: 'T=0.0' },
    { role: 'Human Reviewer', model: 'Browser WebCrypto Client-Side Ed25519 Keyring', t: 'Manual AP2' },
  ];

  return (
    <div className="bg-[#0A0D14] border border-slate-800 rounded-xl p-6 space-y-6 shadow-2xl">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-slate-800 pb-4">
        <div>
          <h2 className="text-base font-bold text-white flex items-center space-x-2">
            <Network className="w-5 h-5 text-indigo-400" />
            <span>Surface 6: Cognitive LLM Mesh &amp; Sovereign Model Deck</span>
          </h2>
          <p className="text-xs text-slate-400 mt-0.5">
            Zero-config local host port discovery, sovereign air-gap isolation, and model assignment matrix
          </p>
        </div>

        <div className="flex items-center space-x-3">
          <button
            onClick={scanRadar}
            disabled={loading}
            className="px-3.5 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-bold rounded-lg flex items-center space-x-1.5 border border-slate-700 transition-colors"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            <span>Scan Host Ports</span>
          </button>

          {/* Sovereign Air-Gap Toggle */}
          <button
            onClick={toggleAirGap}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-bold flex items-center space-x-1.5 border transition-all ${
              airGapped
                ? 'bg-emerald-950 border-emerald-800 text-emerald-300 glow-emerald'
                : 'bg-red-950 border-red-800 text-red-300'
            }`}
          >
            {airGapped ? (
              <>
                <Lock className="w-3.5 h-3.5 text-emerald-400" />
                <span>Sovereign Air-Gap: ENGAGED</span>
              </>
            ) : (
              <>
                <Globe className="w-3.5 h-3.5 text-red-400" />
                <span>Air-Gap: OPEN (Cloud Egress Allowed)</span>
              </>
            )}
          </button>
        </div>
      </div>

      {/* Host Radar Discovery Panel */}
      <div className="bg-[#05070B] border border-slate-800 rounded-xl p-5 space-y-3">
        <h3 className="text-xs font-bold text-slate-300 uppercase tracking-wider flex items-center space-x-2">
          <Terminal className="w-4 h-4 text-cyan-400" />
          <span>Local Host Port Radar (Zero-Config Scanner)</span>
        </h3>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {endpoints.map((ep) => (
            <div
              key={ep.port}
              className="p-3 bg-[#0A0D14] border border-slate-800 rounded-lg flex items-center justify-between text-xs font-mono"
            >
              <div>
                <div className="font-bold text-white">{ep.service}</div>
                <div className="text-slate-500 text-[11px]">127.0.0.1:{ep.port}</div>
              </div>
              <span
                className={`px-2.5 py-0.5 rounded text-[10px] font-bold border ${
                  ep.listening
                    ? 'bg-emerald-950 text-emerald-300 border-emerald-800'
                    : 'bg-slate-900 text-slate-500 border-slate-800'
                }`}
              >
                {ep.listening ? 'ACTIVE LISTENING' : 'OFFLINE'}
              </span>
            </div>
          ))}
        </div>
      </div>

      {/* Tiered Persona Model Assignment Matrix */}
      <div className="bg-[#05070B] border border-slate-800 rounded-xl p-5 space-y-3">
        <h3 className="text-xs font-bold text-slate-300 uppercase tracking-wider flex items-center space-x-2">
          <Sliders className="w-4 h-4 text-indigo-400" />
          <span>Specialist Persona Model Assignment Matrix (Sovereign Local-First)</span>
        </h3>
        <div className="divide-y divide-slate-800 text-xs font-mono">
          {roles.map((r) => (
            <div key={r.role} className="py-2.5 flex items-center justify-between">
              <span className="text-cyan-300 font-bold">{r.role}</span>
              <div className="flex items-center space-x-3">
                <span className="text-slate-300 text-[11px]">{r.model}</span>
                <span className="text-[10px] bg-slate-900 px-2 py-0.5 rounded border border-slate-800 text-amber-400">
                  {r.t}
                </span>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
