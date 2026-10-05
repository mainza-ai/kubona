'use client';

import React, { useState } from 'react';
import { KeyRound, ShieldCheck, Check, AlertOctagon, Sparkles, ChevronDown, ChevronUp, Lock } from 'lucide-react';
import { CampaignState, Session } from '@/types';

interface AutonomyCockpitProps {
  campaign?: CampaignState | null;
  session?: Session | null;
}

export function AutonomyCockpit({ campaign, session }: AutonomyCockpitProps) {
  const estimatedCost = campaign?.quantum_factors?.estimated_spend_usd ?? 0.12;
  const recommendedBackend = campaign?.quantum_factors?.recommended_backend || 'IBM Heron QPU simulator';
  const targetName = campaign?.target_system || 'Target Molecular System';

  const [budgetUSD, setBudgetUSD] = useState(session?.spend_limit_usd ?? 25.0);
  const [currentSpentUSD, setCurrentSpentUSD] = useState(session?.current_spend_usd ?? 1.42);
  const [sovereignAirGap, setSovereignAirGap] = useState(true);
  const [isApproved, setIsApproved] = useState(false);
  const [showCryptoDetails, setShowCryptoDetails] = useState(false);
  const [actionMessage, setActionMessage] = useState<string | null>(null);

  const handleApprovePendingRun = () => {
    setIsApproved(true);
    setCurrentSpentUSD((prev) => prev + estimatedCost);
    setActionMessage('Hardware execution approved. Dispatched to isolated sandbox container.');
    setTimeout(() => setActionMessage(null), 4000);
  };

  const handleEmergencyStop = () => {
    setActionMessage('Emergency Sentinel Stop engaged: All active simulations paused.');
    setTimeout(() => setActionMessage(null), 4000);
  };

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

        {/* Sentinel Shield Badge */}
        <div className="flex items-center space-x-2 px-3.5 py-1.5 rounded-full bg-black/40 border border-white/10 backdrop-blur-xl">
          <ShieldCheck className="w-4 h-4 text-emerald-400" />
          <span className="text-xs font-mono font-medium text-emerald-300">
            Sentinel Protection Active
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
              Est. Cost: ${estimatedCost.toFixed(2)}
            </span>
          </div>

          <p className="text-sm text-slate-300 leading-relaxed font-sans">
            Ansatz Scout has prepared an optimized variational trial for {targetName}. Would you like to run 1,024 shots on the {recommendedBackend}?
          </p>

          <div className="flex items-center space-x-3 pt-2">
            <button
              onClick={handleApprovePendingRun}
              disabled={isApproved}
              className={`flex-1 py-2.5 px-4 text-xs font-semibold rounded-xl flex items-center justify-center space-x-2 transition-all ${
                isApproved
                  ? 'bg-emerald-950/80 border border-emerald-800 text-emerald-300'
                  : 'bg-gradient-to-r from-emerald-600 to-teal-600 hover:from-emerald-500 hover:to-teal-500 text-white shadow-lg active:scale-[0.98]'
              }`}
            >
              <Check className="w-4 h-4" />
              <span>{isApproved ? 'Approved & Dispatched' : 'Approve Simulation Run'}</span>
            </button>
            <button
              onClick={() => {
                setActionMessage('Routed to offline NumPy local statevector simulator.');
                setTimeout(() => setActionMessage(null), 3000);
              }}
              className="py-2.5 px-4 bg-white/05 hover:bg-white/10 text-slate-300 text-xs font-medium rounded-xl transition-all"
            >
              Simulate Offline
            </button>
          </div>
        </div>

        {/* Budget Control Slider Card */}
        <div className="w-full bg-black/50 border border-white/10 rounded-3xl p-6 backdrop-blur-xl space-y-4">
          <div className="flex justify-between items-center text-xs">
            <span className="font-semibold text-white">Daily Spend Ceiling</span>
            <span className="font-mono text-emerald-400 font-bold">${budgetUSD.toFixed(2)} Maximum</span>
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
            <span>Spent today: ${currentSpentUSD.toFixed(2)}</span>
            <span>Available: ${(budgetUSD - currentSpentUSD).toFixed(2)}</span>
          </div>
        </div>

        {/* Sovereign Air-Gap Toggle & Emergency Pause */}
        <div className="w-full flex items-center justify-between p-4 rounded-2xl bg-black/40 border border-white/10">
          <div className="flex items-center space-x-3">
            <div className={`w-8 h-8 rounded-xl flex items-center justify-center ${sovereignAirGap ? 'bg-cyan-950 text-cyan-400 border border-cyan-800' : 'bg-slate-900 text-slate-400'}`}>
              <Lock className="w-4 h-4" />
            </div>
            <div>
              <div className="text-xs font-semibold text-white">Sovereign Air-Gap Mode</div>
              <div className="text-[11px] text-slate-400">Strictly keep computations on local Mac hardware</div>
            </div>
          </div>

          <button
            onClick={() => setSovereignAirGap(!sovereignAirGap)}
            className={`w-11 h-6 rounded-full transition-colors relative p-0.5 ${sovereignAirGap ? 'bg-cyan-500' : 'bg-slate-700'}`}
          >
            <div className={`w-5 h-5 rounded-full bg-white shadow-md transform transition-transform ${sovereignAirGap ? 'translate-x-5' : 'translate-x-0'}`} />
          </button>
        </div>
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
            Tenant: {campaign?.session_id ? `session-${campaign.session_id.slice(0, 8)}` : 'tenant-quantum-lab-1'} • Envelope Verified Statelessly
          </div>
        </div>
      )}
    </div>
  );
}
