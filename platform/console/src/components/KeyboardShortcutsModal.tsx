'use client';

import React, { useEffect } from 'react';
import { X, Command } from 'lucide-react';

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
        { key: '⌘ 0', description: 'Observatory Cockpit Overview' },
        { key: '⌘ 1', description: 'Surface 1: Spatial Research Canvas' },
        { key: '⌘ 2', description: 'Surface 2: 4D Flight Recorder' },
        { key: '⌘ 3', description: 'Surface 3: Epistemic Manifold' },
        { key: '⌘ 4', description: 'Surface 4: Autonomy Cockpit' },
        { key: '⌘ 5', description: 'Surface 5: Execution Fleet Radar' },
        { key: '⌘ 6', description: 'Surface 6: Cognitive Mesh' },
        { key: '⌘ 7', description: 'Surface 7: Citable Results Studio' },
        { key: '⌘ 8', description: 'Surface 8: LLM Mesh & Air-Gap Settings' },
      ],
    },
    {
      category: 'Workspace & Windows',
      shortcuts: [
        { key: '⌘ K', description: 'Open Command Palette / Quick Jump' },
        { key: '⌘ B', description: 'Toggle Campaign Sidebar (Show/Hide)' },
        { key: '⌘ J', description: 'Toggle Research Copilot Drawer' },
        { key: '⌘ I', description: 'About Kubona Scientific Laboratory' },
        { key: '⌘ N', description: 'Create New Research Campaign' },
        { key: '⌘ .', description: 'Emergency Abort (Ring 0 Intercept)' },
        { key: '?', description: 'Show this keyboard shortcuts guide' },
        { key: 'Esc', description: 'Dismiss active dialog or drawer' },
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
