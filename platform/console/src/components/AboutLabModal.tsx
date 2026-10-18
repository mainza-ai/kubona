'use client';

import React, { useEffect, useRef } from 'react';
import Image from 'next/image';
import { X, ShieldCheck, Cpu,  Lock,  Sparkles } from 'lucide-react';

interface AboutLabModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export function AboutLabModal({ isOpen, onClose }: AboutLabModalProps) {
  const modalRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen) {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-md transition-opacity duration-200"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
      role="dialog"
      aria-modal="true"
      aria-labelledby="about-lab-title"
    >
      <div
        ref={modalRef}
        className="w-full max-w-xl bg-[#0C0F17]/95 border border-white/10 rounded-2xl shadow-2xl shadow-black/80 overflow-hidden transform transition-all animate-in fade-in zoom-in-95 duration-150 backdrop-blur-2xl"
      >
        {/* Apple-style Window Header with Traffic Lights */}
        <div className="h-10 px-4 border-b border-white/[0.08] bg-white/[0.02] flex items-center justify-between select-none">
          <div className="flex items-center space-x-2">
            <button
              onClick={onClose}
              aria-label="Close dialog"
              className="w-3 h-3 rounded-full bg-[#FF5F56] hover:brightness-110 active:brightness-90 transition-all flex items-center justify-center group"
            >
              <X className="w-2 h-2 text-black/70 opacity-0 group-hover:opacity-100 transition-opacity" />
            </button>
            <div className="w-3 h-3 rounded-full bg-[#FFBD2E] opacity-75" />
            <div className="w-3 h-3 rounded-full bg-[#27C93F] opacity-75" />
          </div>
          <span className="text-[11px] font-medium text-slate-400 font-sans tracking-wide">
            About Kubona Scientific Laboratory
          </span>
          <div className="w-12" /> {/* Balance spacer */}
        </div>

        {/* Modal Body */}
        <div className="p-6 space-y-6">
          {/* Brand Identity Hero: Crest + Name */}
          <div className="flex items-start space-x-5">
            <div className="relative w-24 h-24 rounded-2xl overflow-hidden ring-1 ring-white/15 shadow-xl bg-black/60 shrink-0">
              <Image
                src="/kubona_crest.jpeg"
                alt="Kubona Scientific Laboratory Crest"
                fill
                sizes="96px"
                className="object-cover"
                priority
              />
            </div>

            <div className="flex-1 space-y-1">
              <div className="flex items-center space-x-2">
                <h2 id="about-lab-title" className="text-xl font-bold text-white tracking-tight">
                  Kubona
                </h2>
                <span className="text-[10px] font-mono font-bold px-2 py-0.5 rounded-full bg-cyan-950/80 text-cyan-300 border border-cyan-800/60">
                  OBSERVATORY PRO
                </span>
              </div>
              <p className="text-xs font-semibold text-cyan-400/90 tracking-wide uppercase">
                Tonga (Zambia) — <span className="italic">To see</span>
              </p>
              <p className="text-xs text-slate-300 leading-relaxed pt-1">
                An open-source, sovereign, MCP-native quantum computing and autonomous AI science discovery platform for frontier empirical exploration.
              </p>
            </div>
          </div>

          {/* System Architecture Specifications Grid */}
          <div className="grid grid-cols-2 gap-3 text-xs">
            <div className="p-3.5 rounded-xl bg-white/[0.03] border border-white/10 space-y-1">
              <div className="flex items-center space-x-1.5 text-slate-200 font-semibold">
                <ShieldCheck className="w-3.5 h-3.5 text-emerald-400" />
                <span>Cryptographic Posture</span>
              </div>
              <div className="font-mono text-xs text-slate-200 font-medium">
                Ed25519 AP2 Mandates (wired)
              </div>
              <div className="text-[11px] text-slate-300">
                ML-KEM-768 hybrid encapsulation staged — not wired, no recipient registry yet
              </div>
            </div>

            <div className="p-3.5 rounded-xl bg-white/[0.03] border border-white/10 space-y-1">
              <div className="flex items-center space-x-1.5 text-slate-200 font-semibold">
                <Lock className="w-3.5 h-3.5 text-amber-400" />
                <span>Workload &amp; Secrets</span>
              </div>
              <div className="font-mono text-xs text-slate-200 font-medium">
                OpenBao Dynamic TTL Leases
              </div>
              <div className="text-[11px] text-slate-300">
                Buzz Information Flow Control
              </div>
            </div>

            <div className="p-3.5 rounded-xl bg-white/[0.03] border border-white/10 space-y-1">
              <div className="flex items-center space-x-1.5 text-slate-200 font-semibold">
                <Cpu className="w-3.5 h-3.5 text-cyan-400" />
                <span>Quantum &amp; Classical Engines</span>
              </div>
              <div className="font-mono text-xs text-slate-200 font-medium">
                PySCF FCI + Qiskit Aer
              </div>
              <div className="text-[11px] text-slate-300">
                PennyLane QPD &amp; Mitiq ZNE
              </div>
            </div>

            <div className="p-3.5 rounded-xl bg-white/[0.03] border border-white/10 space-y-1">
              <div className="flex items-center space-x-1.5 text-slate-200 font-semibold">
                <Sparkles className="w-3.5 h-3.5 text-indigo-400" />
                <span>Cognitive Intelligence</span>
              </div>
              <div className="font-mono text-xs text-slate-200 font-medium">
                Env-Driven Provider Routing (NIM / Local)
              </div>
              <div className="text-[11px] text-slate-300">
                4-Turn Epistemic Tribunal Loop
              </div>
            </div>
          </div>

          {/* Version & Build Bar — static snapshot with as-of dates (no version
              endpoint wired yet; do not present as live). */}
          <div className="p-3.5 rounded-xl bg-black/60 border border-white/10 flex items-center justify-between text-xs font-mono text-slate-300">
            <div>
              <span className="text-slate-300 font-medium">Core Daemon:</span>{' '}
              <strong className="text-white font-bold">v0.2.0-ts-go</strong> (as of 2026-10-12)
            </div>
            <div>
              <span className="text-slate-300 font-medium">Bridge:</span>{' '}
              <strong className="text-cyan-300 font-bold">gRPC</strong> (as of 2026-10-12)
            </div>
            <div>
              <span className="text-slate-300 font-medium">License:</span>{' '}
              <strong className="text-emerald-300 font-bold">Apache-2.0</strong>
            </div>
          </div>
        </div>

        {/* Modal Footer */}
        <div className="h-12 px-6 border-t border-white/[0.08] bg-white/[0.02] flex items-center justify-between">
          <span className="text-xs text-slate-300">
            Press <kbd className="px-1.5 py-0.5 rounded bg-white/10 font-mono text-[10px] text-slate-200 border border-white/15">Esc</kbd> to dismiss
          </span>
          <button
            onClick={onClose}
            className="px-4 py-1.5 rounded-lg bg-white/10 hover:bg-white/15 active:scale-95 text-xs font-semibold text-white transition-all focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none"
          >
            Done
          </button>
        </div>
      </div>
    </div>
  );
}
