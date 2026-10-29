'use client';

import React, { useEffect } from 'react';
import { X, Command } from 'lucide-react';
import { SHORTCUTS } from '@/lib/shortcuts';

interface KeyboardShortcutsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export function KeyboardShortcutsModal({ isOpen, onClose }: KeyboardShortcutsModalProps) {
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

  const shortcutGroups = [
    {
      category: 'Surfaces Navigation',
      shortcuts: [
        { key: SHORTCUTS.livingCanvas, description: 'Active Research Arena' },
        { key: SHORTCUTS.flightRecorder, description: '4D Flight Recorder' },
        { key: SHORTCUTS.epistemicOrb, description: 'Epistemic Manifold' },
        { key: SHORTCUTS.livingNotebook, description: 'Merkle Proofs & Papers' },
        { key: SHORTCUTS.fleetRadar, description: 'Quantum Fleet Radar' },
        { key: SHORTCUTS.cognitiveMesh, description: 'Local Sovereign Mesh' },
      ],
    },
    {
      category: 'Deep Links (Surface Tabs)',
      shortcuts: [
        { key: SHORTCUTS.sentinelGuardrails, description: 'Autonomy Cockpit & Guardrails' },
        { key: SHORTCUTS.previewGallery, description: 'Preview Gallery' },
        { key: SHORTCUTS.huggingFaceHub, description: 'Hugging Face Model Hub' },
        { key: SHORTCUTS.laboratories, description: 'Experimental Tenant Labs' },
        { key: SHORTCUTS.sovereignAirGap, description: 'Sovereign Settings' },
        { key: SHORTCUTS.tribunalRoom, description: 'Epistemic Tribunal Room' },
      ],
    },
    {
      category: 'Workspace & Windows',
      shortcuts: [
        { key: SHORTCUTS.commandPalette, description: 'Open Command Palette / Quick Jump' },
        { key: SHORTCUTS.copilot, description: 'Toggle Kubona Copilot' },
        { key: SHORTCUTS.about, description: 'About Kubona Scientific Laboratory' },
        { key: SHORTCUTS.newCampaign, description: 'Create New Research Campaign' },
        { key: SHORTCUTS.emergencyAbort, description: 'Emergency Abort (Ring 0 Intercept)' },
        { key: SHORTCUTS.help, description: 'Show this keyboard shortcuts guide' },
        { key: SHORTCUTS.dismiss, description: 'Dismiss active dialog or drawer' },
      ],
    },
  ];

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-md animate-in fade-in duration-150"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
      role="dialog"
      aria-modal="true"
      aria-labelledby="shortcuts-title"
    >
      <div className="w-full max-w-lg bg-[#0E111A]/95 border border-white/10 rounded-2xl shadow-2xl shadow-black/90 overflow-hidden backdrop-blur-2xl animate-in zoom-in-95 duration-150">
        <div className="h-12 px-5 border-b border-white/[0.08] flex items-center justify-between bg-white/[0.02]">
          <div className="flex items-center space-x-2">
            <Command className="w-4 h-4 text-cyan-400" />
            <h2 id="shortcuts-title" className="text-sm font-semibold text-white">
              Keyboard Shortcuts
            </h2>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/10 transition-colors"
            aria-label="Close"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-5 space-y-5 max-h-[70vh] overflow-y-auto">
          {shortcutGroups.map((group) => (
            <div key={group.category} className="space-y-2">
              <h3 className="text-[11px] font-semibold text-slate-400 uppercase tracking-wider font-mono">
                {group.category}
              </h3>
              <div className="space-y-1.5">
                {group.shortcuts.map((sc) => (
                  <div
                    key={sc.key}
                    className="flex items-center justify-between p-2 rounded-xl bg-white/[0.03] border border-white/[0.04] text-xs"
                  >
                    <span className="text-slate-200">{sc.description}</span>
                    <kbd className="px-2 py-0.5 rounded-md bg-white/10 text-[11px] font-mono text-cyan-300 border border-white/10 shadow-sm">
                      {sc.key}
                    </kbd>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>

        <div className="h-10 px-5 border-t border-white/[0.08] bg-white/[0.02] flex items-center justify-end">
          <button
            onClick={onClose}
            className="px-4 py-1 rounded-lg bg-white/10 hover:bg-white/15 text-xs font-semibold text-white transition-all"
          >
            Got it
          </button>
        </div>
      </div>
    </div>
  );
}
