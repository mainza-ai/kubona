'use client';

import React, { useEffect, useRef } from 'react';
import Image from 'next/image';
import { X, ShieldCheck, Cpu,  Lock,  Sparkles } from 'lucide-react';
import { useFocusTrap } from '@/lib/useFocusTrap';

// Build version is injected by the console image (Dockerfile.console ->
// NEXT_PUBLIC_KUBONA_VERSION). Local dev falls back to "dev"; a literal
// version in source is a stale claim, not a build fact.
const BUILD_VERSION = process.env.NEXT_PUBLIC_KUBONA_VERSION || 'dev';

interface AboutLabModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export function AboutLabModal({ isOpen, onClose }: AboutLabModalProps) {
  const modalRef = useRef<HTMLDivElement>(null);

  // Wave-2 a11y: trap Tab inside the dialog, land focus on entry, restore
  // the invoker on close. Escape handling below is unchanged.
  useFocusTrap(isOpen, modalRef);

  useEffect(() => {
    if (typeof window === 'undefined') return;
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
        className="w-full max-w-xl bg-float border border-border-strong rounded-card shadow-[var(--shadow-float)] overflow-hidden transform transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] animate-in fade-in zoom-in-95 duration-150 backdrop-blur-2xl"
      >
        {/* Apple-style Window Header with Traffic Lights */}
        <div className="h-10 px-4 border-b border-border bg-surface flex items-center justify-between select-none">
          <div className="flex items-center space-x-2">
            <button
              onClick={onClose}
              aria-label="Close dialog"
              className="w-3 h-3 rounded-capsule bg-[#FF5F56] hover:brightness-110 active:brightness-90 transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] flex items-center justify-center group"
            >
              <X className="w-2 h-2 text-black/70 opacity-0 group-hover:opacity-100 transition-opacity" />
            </button>
            <div className="w-3 h-3 rounded-capsule bg-[#FFBD2E] opacity-75" />
            <div className="w-3 h-3 rounded-capsule bg-[#27C93F] opacity-75" />
          </div>
          <span className="text-[12px] font-medium text-muted font-sans tracking-wide">
            About Kubona Scientific Laboratory
          </span>
          <div className="w-12" /> {/* Balance spacer */}
        </div>

        {/* Modal Body */}
        <div className="p-6 space-y-6">
          {/* Brand Identity Hero: Crest + Name */}
          <div className="flex items-start space-x-5">
            <div className="relative w-24 h-24 rounded-card overflow-hidden ring-1 ring-border-strong shadow-xl bg-field shrink-0">
              <Image
                src="/assets/milimo_kubona_logo.png"
                alt="Kubona logo"
                fill
                sizes="96px"
                className="object-contain p-1"
                priority
              />
            </div>

            <div className="flex-1 space-y-1">
              <div className="flex items-center space-x-2">
                <h2 id="about-lab-title" className="text-title-small font-bold text-text tracking-tight">
                  Kubona
                </h2>
                <span className="text-[12px] font-mono font-bold px-2 py-0.5 rounded-capsule bg-info-wash text-info-ink border border-info-edge">
                  OBSERVATORY PRO
                </span>
              </div>
              <p className="text-label font-semibold text-accent-ink tracking-wide uppercase">
                Tonga (Zambia) — <span className="italic">To see</span>
              </p>
              <p className="text-label text-label-secondary leading-relaxed pt-1">
                An open-source, sovereign, MCP-native quantum computing and autonomous AI science discovery platform for frontier empirical exploration.
              </p>
            </div>
          </div>

          {/* System Architecture Specifications Grid */}
          <div className="grid grid-cols-2 gap-3 text-label">
            <div className="p-3.5 rounded-row bg-card border border-border space-y-1">
              <div className="flex items-center space-x-1.5 text-text font-semibold">
                <ShieldCheck className="w-3.5 h-3.5 text-success-ink" />
                <span>Cryptographic Posture</span>
              </div>
              <div className="font-mono text-label text-label-secondary font-medium">
                Ed25519 AP2 Mandates (wired)
              </div>
              <div className="text-[12px] text-muted">
                ML-KEM-768 hybrid encapsulation staged — not wired, no recipient registry yet
              </div>
            </div>

            <div className="p-3.5 rounded-row bg-card border border-border space-y-1">
              <div className="flex items-center space-x-1.5 text-text font-semibold">
                <Lock className="w-3.5 h-3.5 text-warning-ink" />
                <span>Workload &amp; Secrets</span>
              </div>
              <div className="font-mono text-label text-label-secondary font-medium">
                OpenBao Dynamic TTL Leases
              </div>
              <div className="text-[12px] text-muted">
                Buzz Information Flow Control
              </div>
              <div className="text-[12px] text-warning-ink font-mono">
                Vault is dev-compose only; IFC is partial. Neither is claimed as deployed here.
              </div>
            </div>

            <div className="p-3.5 rounded-row bg-card border border-border space-y-1">
              <div className="flex items-center space-x-1.5 text-text font-semibold">
                <Cpu className="w-3.5 h-3.5 text-accent-ink" />
                <span>Quantum &amp; Classical Engines</span>
              </div>
              <div className="font-mono text-label text-label-secondary font-medium">
                PySCF FCI + Qiskit Aer
              </div>
              <div className="text-[12px] text-muted">
                PennyLane QPD &amp; Mitiq ZNE
              </div>
            </div>

            <div className="p-3.5 rounded-row bg-card border border-border space-y-1">
              <div className="flex items-center space-x-1.5 text-text font-semibold">
                <Sparkles className="w-3.5 h-3.5 text-accent-ink" />
                <span>Cognitive Intelligence</span>
              </div>
              <div className="font-mono text-label text-label-secondary font-medium">
                Env-Driven Provider Routing (NIM / Local)
              </div>
              <div className="text-[12px] text-muted">
                4-Turn Epistemic Tribunal Loop
              </div>
            </div>
          </div>

          {/* Build version is injected at image build time. There is no live
              version endpoint yet; do not present this as a live probe. */}
          <div className="p-3.5 rounded-row bg-field border border-border flex items-center justify-between text-label font-mono text-label-secondary">
            <div>
              <span className="text-muted font-medium">Build:</span>{' '}
              <strong className="text-text font-bold">{BUILD_VERSION}</strong>
            </div>
            <div>
              <span className="text-muted font-medium">Bridge:</span>{' '}
              <strong className="text-info-ink font-bold">gRPC</strong>
            </div>
            <div>
              <span className="text-muted font-medium">License:</span>{' '}
              <strong className="text-success-ink font-bold">Apache-2.0</strong>
            </div>
          </div>
        </div>

        {/* Modal Footer */}
        <div className="h-12 px-6 border-t border-border bg-surface flex items-center justify-between">
          <span className="text-label text-muted">
            Press <kbd className="px-1.5 py-0.5 rounded-chip bg-fill font-mono text-[12px] text-label-secondary border border-border">Esc</kbd> to dismiss
          </span>
          <button
            onClick={onClose}
            className="px-4 py-1.5 rounded-control-compact bg-fill hover:bg-fill-hover active:scale-95 text-label font-semibold text-text transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none"
          >
            Done
          </button>
        </div>
      </div>
    </div>
  );
}
