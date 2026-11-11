'use client';

import React, { useEffect, useRef } from 'react';
import { X, Command } from 'lucide-react';
import { SHORTCUTS } from '@/lib/shortcuts';
import { useFocusTrap } from '@/lib/useFocusTrap';

interface KeyboardShortcutsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export function KeyboardShortcutsModal({ isOpen, onClose }: KeyboardShortcutsModalProps) {
  const panelRef = useRef<HTMLDivElement>(null);

  // Wave-2 a11y: trap Tab inside the dialog, land focus on entry, restore
  // the invoker on close. Escape handling below is unchanged.
  useFocusTrap(isOpen, panelRef);

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
        { key: SHORTCUTS.previewGallery, description: 'Scientific Widgets' },
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
        { key: SHORTCUTS.experimentLab, description: 'Toggle Experiment & Discovery Lab' },
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
      <div ref={panelRef} className="w-full max-w-lg bg-[var(--color-float)] border border-[var(--color-border-strong)] rounded-2xl shadow-[var(--shadow-float)] overflow-hidden backdrop-blur-2xl animate-in zoom-in-95 duration-150">
        <div className="h-12 px-5 border-b border-[var(--color-border)] flex items-center justify-between bg-[var(--color-surface)]">
          <div className="flex items-center space-x-2">
            <Command className="w-4 h-4 text-[var(--color-accent-ink)]" />
            <h2 id="shortcuts-title" className="text-sm font-semibold text-[var(--color-text)]">
              Keyboard Shortcuts
            </h2>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-fill-hover)] transition-colors"
            aria-label="Close"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-5 space-y-5 max-h-[70vh] overflow-y-auto">
          {shortcutGroups.map((group) => (
            <div key={group.category} className="space-y-2">
              <h3 className="text-[12px] font-semibold text-[var(--color-muted)] uppercase tracking-wider font-mono">
                {group.category}
              </h3>
              <div className="space-y-1.5">
                {group.shortcuts.map((sc) => (
                  <div
                    key={sc.key}
                    className="flex items-center justify-between p-2 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)] text-xs"
                  >
                    <span className="text-[var(--color-label-secondary)]">{sc.description}</span>
                    <kbd className="px-2 py-0.5 rounded-md bg-[var(--color-fill)] text-[12px] font-mono text-[var(--color-info-ink)] border border-[var(--color-border)] shadow-sm">
                      {sc.key}
                    </kbd>
                  </div>
                ))}
              </div>
            </div>
          ))}
          <div className="space-y-2">
            <h3 className="text-[12px] font-semibold text-[var(--color-muted)] uppercase tracking-wider font-mono">
              Deep links
            </h3>
            <div className="p-3 rounded-xl bg-[var(--color-card)] border border-[var(--color-border)] text-xs text-[var(--color-label-secondary)]">
              Share or bookmark <code className="font-mono text-[var(--color-info-ink)]">?surface=</code>
              <span className="font-mono">arena|recorder|manifold|fleet|proofs|mesh</span> with{' '}
              <code className="font-mono text-[var(--color-info-ink)]">?tab=</code>
              <span className="font-mono">manuscripts|tribunal|gallery|circuits</span> (proofs),{' '}
              <span className="font-mono">mesh|guardrails|models|labs|settings</span> (mesh),{' '}
              <span className="font-mono">orbital|proof-dag</span> (arena).
            </div>
          </div>
        </div>

        <div className="h-10 px-5 border-t border-[var(--color-border)] bg-[var(--color-surface)] flex items-center justify-end">
          <button
            onClick={onClose}
            className="px-4 py-1 rounded-lg bg-[var(--color-fill)] hover:bg-[var(--color-fill-hover)] text-xs font-semibold text-[var(--color-text)] transition-all"
          >
            Got it
          </button>
        </div>
      </div>
    </div>
  );
}
