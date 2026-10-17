'use client';

import React, { useState, useEffect, useRef } from 'react';
import {
  Search,
  Orbit,
  PlayCircle,
  Compass,
  Radio,
  Network,
  BookOpen,
  Plus,
  Zap,
  Info,
  Command,
} from 'lucide-react';
import { SHORTCUTS } from '@/lib/shortcuts';

interface CommandPaletteProps {
  isOpen: boolean;
  onClose: () => void;
  onSelectSurface: (surfaceId: string) => void;
  onNewCampaign: () => void;
  onEmergencyAbort: () => void;
  onOpenAbout: () => void;
}

export function CommandPalette({
  isOpen,
  onClose,
  onSelectSurface,
  onNewCampaign,
  onEmergencyAbort,
  onOpenAbout,
}: CommandPaletteProps) {
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  const items = [
    {
      id: 'arena',
      title: 'Active Research Arena (Living Swarm Universe)',
      category: 'Surfaces',
      shortcut: SHORTCUTS.livingCanvas,
      icon: Orbit,
      action: () => onSelectSurface('arena'),
    },
    {
      id: 'recorder',
      title: '4D Flight Recorder (Scrub Discovery Story)',
      category: 'Surfaces',
      shortcut: SHORTCUTS.flightRecorder,
      icon: PlayCircle,
      action: () => onSelectSurface('recorder'),
    },
    {
      id: 'manifold',
      title: 'Epistemic Manifold (Curvature & Belief Topology)',
      category: 'Surfaces',
      shortcut: SHORTCUTS.epistemicOrb,
      icon: Compass,
      action: () => onSelectSurface('manifold'),
    },
    {
      id: 'fleet',
      title: 'Quantum Fleet Radar (QuEra, IBM & Slurm HPC)',
      category: 'Surfaces',
      shortcut: SHORTCUTS.fleetRadar,
      icon: Radio,
      action: () => onSelectSurface('fleet'),
    },
    {
      id: 'proofs',
      title: 'Merkle Proofs & Papers (Lean 4 ASTs & LaTeX)',
      category: 'Surfaces',
      shortcut: SHORTCUTS.livingNotebook,
      icon: BookOpen,
      action: () => onSelectSurface('proofs'),
    },
    {
      id: 'mesh',
      title: 'Local Sovereign Mesh (MLX · Ollama · Air-Gap)',
      category: 'Surfaces',
      shortcut: SHORTCUTS.cognitiveMesh,
      icon: Network,
      action: () => onSelectSurface('mesh'),
    },
    {
      id: 'new-campaign',
      title: 'New Research Campaign',
      category: 'Actions',
      shortcut: SHORTCUTS.newCampaign,
      icon: Plus,
      action: onNewCampaign,
    },
    {
      id: 'about-lab',
      title: 'About Kubona Scientific Laboratory',
      category: 'Actions',
      shortcut: SHORTCUTS.about,
      icon: Info,
      action: onOpenAbout,
    },
    {
      id: 'emergency-abort',
      title: 'Emergency Abort (Ring 0 Intercept)',
      category: 'Safety',
      shortcut: SHORTCUTS.emergencyAbort,
      icon: Zap,
      action: () => {
        if (confirm('EMERGENCY ABORT: Settle every unsettled job in this session as failed?')) {
          onEmergencyAbort();
        }
      },
    },
  ];

  const filtered = items.filter((item) =>
    item.title.toLowerCase().includes(query.toLowerCase()) ||
    item.category.toLowerCase().includes(query.toLowerCase())
  );

  useEffect(() => {
    if (isOpen) {
      const timer = setTimeout(() => {
        setQuery('');
        setSelectedIndex(0);
        inputRef.current?.focus();
      }, 0);
      return () => clearTimeout(timer);
    }
  }, [isOpen]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!isOpen) return;

      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setSelectedIndex((prev) => (prev + 1) % (filtered.length || 1));
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setSelectedIndex((prev) => (prev - 1 + filtered.length) % (filtered.length || 1));
      } else if (e.key === 'Enter') {
        e.preventDefault();
        if (filtered[selectedIndex]) {
          filtered[selectedIndex].action();
          onClose();
        }
      } else if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, filtered, selectedIndex, onClose]);

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-24 p-4 bg-black/60 backdrop-blur-md animate-in fade-in duration-150"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
      role="dialog"
      aria-modal="true"
    >
      <div className="w-full max-w-lg bg-[#0E111A]/95 border border-white/10 rounded-2xl shadow-2xl shadow-black/90 overflow-hidden backdrop-blur-2xl animate-in zoom-in-95 duration-150">
        {/* Search Input Bar with Apple Spotlight aesthetics */}
        <div className="h-14 px-4 border-b border-white/[0.08] flex items-center space-x-3 bg-white/[0.02]">
          <Search className="w-5 h-5 text-slate-400 shrink-0" />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setSelectedIndex(0);
            }}
            placeholder="Type a surface name or command..."
            className="flex-1 bg-transparent text-sm text-white placeholder-slate-500 focus:outline-none"
            aria-label="Search surfaces and actions"
          />
          <kbd className="px-2 py-0.5 rounded bg-white/10 text-[10px] font-mono text-slate-400 border border-white/10">
            Esc
          </kbd>
        </div>

        {/* Results List */}
        <div className="max-h-80 overflow-y-auto p-2 space-y-1">
          {filtered.length === 0 ? (
            <div className="py-8 text-center text-xs text-slate-300 font-medium">
              No matching surfaces or commands found
            </div>
          ) : (
            filtered.map((item, idx) => {
              const Icon = item.icon;
              const isSelected = idx === selectedIndex;
              return (
                <button
                  key={item.id}
                  onClick={() => {
                    item.action();
                    onClose();
                  }}
                  onMouseEnter={() => setSelectedIndex(idx)}
                  className={`w-full px-3 py-2.5 rounded-xl text-xs flex items-center justify-between transition-all text-left ${
                    isSelected
                      ? 'bg-cyan-500/15 text-white border border-cyan-500/30 shadow-sm'
                      : 'text-slate-300 hover:bg-white/[0.04] border border-transparent'
                  }`}
                >
                  <div className="flex items-center space-x-3">
                    <div
                      className={`p-1.5 rounded-lg ${
                        isSelected ? 'bg-cyan-500 text-black' : 'bg-white/5 text-slate-300'
                      }`}
                    >
                      <Icon className="w-4 h-4" />
                    </div>
                    <div>
                      <div className="font-medium text-slate-100">{item.title}</div>
                      <div className="text-[11px] text-slate-300 font-medium">{item.category}</div>
                    </div>
                  </div>
                  {item.shortcut && (
                    <kbd className="px-2 py-0.5 rounded bg-white/10 text-[10px] font-mono text-slate-200 border border-white/15 font-semibold">
                      {item.shortcut}
                    </kbd>
                  )}
                </button>
              );
            })
          )}
        </div>

        {/* Footer with Hint */}
        <div className="h-9 px-4 border-t border-white/[0.08] bg-white/[0.03] flex items-center justify-between text-xs text-slate-300 font-medium">
          <span>Navigate with ↑ ↓ · Select with ↵</span>
          <span className="flex items-center space-x-1.5">
            <Command className="w-3.5 h-3.5 text-slate-300" />
            <span>K to toggle</span>
          </span>
        </div>
      </div>
    </div>
  );
}
