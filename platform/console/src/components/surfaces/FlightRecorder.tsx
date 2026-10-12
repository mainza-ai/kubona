'use client';

import React, { useState, useEffect, useMemo } from 'react';
import { 
  Play, Pause, RotateCcw, CheckCircle, Sparkles, ChevronDown, ChevronUp, FileCode
} from 'lucide-react';
import { Loop, LedgerEvent, CampaignState } from '@/types';

interface FlightRecorderProps {
  loops?: Loop[];
  events?: LedgerEvent[];
  campaign?: CampaignState | null;
}

interface StoryMilestone {
  step: number;
  title: string;
  verdict: 'keep' | 'discard' | 'baseline';
  summary: string;
  metric: number;
  deltaMHa: number;
  codeDiff?: string;
}

export function FlightRecorder({ loops = [], events = [], campaign }: FlightRecorderProps) {
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentStepIndex, setCurrentStepIndex] = useState(3);
  const [showCodeDiff, setShowCodeDiff] = useState(false);
  const [isStepping, setIsStepping] = useState(false);

  const trajectory = campaign?.trajectory;
  const fciEnergy = campaign?.classical_anchor?.fci_exact_energy;
  const chemLimit = campaign?.classical_anchor?.chemical_accuracy_limit ?? 1.6;
  const targetSystem = campaign?.target_system || 'H₂';
  const ansatzFamily = campaign?.quantum_factors?.ansatz_family || 'TwoLocal';

  // Dynamic narrative milestones derived from campaign trajectory and Karpathy loops
  const milestones: StoryMilestone[] = useMemo(() => {
    if (trajectory && trajectory.length > 0) {
      const fci = fciEnergy ?? trajectory[trajectory.length - 1].energy;
      return trajectory.map((t, idx) => {
        const deltaMHa = Math.abs(t.energy - fci) * 1000;
        const isDiscard = t.watchdog?.toUpperCase().includes('TRIP') || t.watchdog?.toUpperCase().includes('DISCARD');
        const isBaseline = idx === 0;
        const isBreakthrough = deltaMHa <= chemLimit;
        // Measured points (e.g. Krylov energies folded from real jobs)
        // are observations, not optimizer steps: label them as such so
        // they never read as variational convergence.
        const isMeasured = (t.watchdog || '').toLowerCase().startsWith('measured');

        let title = `Variational Optimization Step ${t.step}`;
        let verdict: 'keep' | 'discard' | 'baseline' = 'keep';
        if (isMeasured) {
          title = `Measured Point ${t.step} (${t.watchdog})`;
        } else if (isBaseline) {
          title = `Baseline ${targetSystem} Hypothesis`;
          verdict = 'baseline';
        } else if (isDiscard) {
          title = 'Ansatz Mutation Discarded';
          verdict = 'discard';
        } else if (isBreakthrough) {
          title = 'Chemical Accuracy Achieved';
          verdict = 'keep';
        }

        const summary = isMeasured
          ? `Directly measured energy ${t.energy.toFixed(5)} Ha from job execution (${t.watchdog}) — an observation, not an optimizer step.`
          : isBaseline
          ? `Initial parameterized ansatz evaluated for ${targetSystem}. Baseline energy: ${t.energy.toFixed(5)} Ha.`
          : isDiscard
          ? `Disconfirmation Officer triggered watchdog (${t.watchdog}). Mutation discarded to prevent barren plateau.`
          : isBreakthrough
          ? `Energy converged to ${t.energy.toFixed(5)} Ha. Discrepancy against exact FCI is within ${deltaMHa.toFixed(2)} mHa chemical threshold.`
          : `Ansatz parameter vector updated. Energy lowered to ${t.energy.toFixed(5)} Ha (grad norm: ${t.grad_norm.toFixed(4)}).`;

        const thetaStr = t.theta && t.theta.length > 0
          ? t.theta.map((v) => v.toFixed(3)).join(', ')
          : 'not recorded';

        const codeDiff = isMeasured
          ? `  # Measured observation (${t.watchdog})\n+ E = ${t.energy.toFixed(5)} Ha (Δ = ${deltaMHa.toFixed(2)} mHa)\n+ no optimizer state: not a gradient step`
          : isBaseline
          ? `+ # Initial Ansatz: ${ansatzFamily}\n+ theta = [${thetaStr}]\n+ E_init = ${t.energy.toFixed(5)} Ha`
          : isDiscard
          ? `- # Watchdog trip: ${t.watchdog}\n- Reverted parameters\n+ Retained stable prior manifold`
          : `  # Gradient Step ${t.step}\n- grad_norm: ${t.grad_norm.toFixed(4)}\n+ theta = [${thetaStr}]\n+ E = ${t.energy.toFixed(5)} Ha (Δ = ${deltaMHa.toFixed(2)} mHa)`;

        return {
          step: t.step,
          title,
          verdict,
          summary,
          metric: t.loss,
          deltaMHa,
          codeDiff,
        };
      });
    }

    // Fresh campaign with no optimization steps executed yet: state what
    // exists (the anchor, if computed) and nothing else.
    if (trajectory !== undefined && trajectory.length === 0) {
      const anchorKnown = fciEnergy != null;
      return [
        {
          step: 0,
          title: `Initial ${targetSystem} Hypothesis`,
          verdict: 'baseline',
          summary: anchorKnown
            ? `Campaign initialized for ${targetSystem}. Classical anchor established at ${fciEnergy.toFixed(5)} Ha. Awaiting first variational trial or Research Tribunal deliberation.`
            : `Campaign initialized for ${targetSystem}. No anchor computed and no trials run yet — nothing measured.`,
          metric: 0.0,
          deltaMHa: -1,
          codeDiff: `+ # Target Molecule: ${targetSystem}\n+ # Classical FCI Anchor: ${anchorKnown ? fciEnergy.toFixed(5) + ' Ha' : 'UNVERIFIED'}\n+ # Status: Initial Hypothesis Formulated (Awaiting First Step)`,
        },
      ];
    }

    // No campaign and no trajectory: a single explicitly UNVERIFIED
    // placeholder. Curated fiction (invented energies, metrics, verdicts)
    // is forbidden here — an empty recorder states that it is empty.
    return [
      {
        step: 0,
        title: 'No telemetry recorded',
        verdict: 'baseline',
        summary: 'No campaign is attached: no trials ran, no anchor computed, no verdicts rendered. Run a campaign to populate this recorder.',
        metric: 0.0,
        deltaMHa: -1,
        codeDiff: '+ # Status: UNVERIFIED — awaiting first campaign',
      },
    ];
  }, [trajectory, fciEnergy, chemLimit, targetSystem, ansatzFamily]);

  const safeIndex = Math.min(Math.max(0, currentStepIndex), Math.max(0, milestones.length - 1));
  const activeMilestone = milestones[safeIndex] || milestones[0];

  // Playback timer
  useEffect(() => {
    let timer: NodeJS.Timeout;
    if (isPlaying) {
      timer = setInterval(() => {
        setCurrentStepIndex((prev) => {
          if (prev >= milestones.length - 1) {
            setIsPlaying(false);
            return prev;
          }
          return prev + 1;
        });
      }, 2500);
    }
    return () => clearInterval(timer);
  }, [isPlaying, milestones.length]);

  // Playback-only stepping: scrubs the recorded narrative, dispatches
  // nothing. The label says preview so it can never read as an experiment.
  const handleStepPreview = async () => {
    setIsStepping(true);
    try {
      await new Promise((r) => setTimeout(r, 400));
      if (currentStepIndex < milestones.length - 1) {
        setCurrentStepIndex((prev) => prev + 1);
      }
    } finally {
      setIsStepping(false);
    }
  };

  return (
    <div className="relative min-h-[640px] rounded-3xl overflow-hidden bg-gradient-to-b from-[#090C13] via-[#0B0F19] to-[#07090E] border border-white/[0.08] shadow-2xl flex flex-col justify-between p-8 select-none">
      {/* Top Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-white/[0.08] pb-5">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-2xl bg-white/[0.05] border border-white/10 backdrop-blur-xl flex items-center justify-center shadow-inner">
            <Sparkles className="w-5 h-5 text-cyan-400" />
          </div>
          <div>
            <h2 className="text-base font-semibold text-white tracking-tight">
              Discovery Flight Recorder · {campaign?.target_system || 'H₂'}
            </h2>
            <p className="text-xs text-slate-300 mt-0.5">
              Narrative playback of autonomous hypothesis evolution ({loops.length} loops, {events.length} ledger events)
            </p>
          </div>
        </div>

        {/* Step Playback Preview Button */}
        <button
          onClick={handleStepPreview}
          disabled={isStepping}
          title="Scrub forward through the recorded narrative (dispatches nothing)"
          className="px-4 py-2 rounded-xl bg-white/[0.06] hover:bg-white/[0.1] border border-white/10 active:scale-[0.98] text-white text-xs font-semibold flex items-center space-x-2 transition-all shadow-md disabled:opacity-50"
        >
          <Sparkles className="w-3.5 h-3.5" />
          <span>{isStepping ? 'Scrubbing...' : 'Preview Next Step'}</span>
        </button>
      </div>

      {/* Center Narrative Story Card */}
      <div className="flex-1 flex flex-col items-center justify-center my-8 max-w-xl mx-auto w-full">
        <div className="w-full bg-black/60 border border-white/10 rounded-3xl p-7 backdrop-blur-2xl shadow-2xl space-y-5">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-mono font-bold px-3 py-1 rounded-full bg-white/[0.08] text-slate-200 border border-white/15">
              Iteration {activeMilestone.step} of {milestones.length}
            </span>

            <span className={`text-[11px] font-mono font-bold px-3 py-1 rounded-full flex items-center space-x-1.5 ${
              activeMilestone.verdict === 'keep' || activeMilestone.verdict === 'baseline'
                ? 'bg-emerald-950/80 text-emerald-300 border border-emerald-800/60'
                : 'bg-amber-950/80 text-amber-300 border border-amber-800/60'
            }`}>
              <CheckCircle className="w-3.5 h-3.5" />
              <span>{activeMilestone.verdict === 'keep' ? 'Champion Promoted' : activeMilestone.verdict === 'baseline' ? 'Baseline Established' : 'Hypothesis Discarded'}</span>
            </span>
          </div>

          <div>
            <h3 className="text-lg font-semibold text-white tracking-tight">
              {activeMilestone.title}
            </h3>
            <p className="text-sm text-slate-200 leading-relaxed mt-2 font-sans">
              {activeMilestone.summary}
            </p>
          </div>

          {/* Key physical metric readout */}
          <div className="flex items-center justify-between pt-3 border-t border-white/[0.08] text-xs font-mono">
            <span className="text-slate-300 font-medium">Chemical Discrepancy:</span>
            {activeMilestone.deltaMHa < 0 ? (
              <span className="font-bold text-slate-400">Δ = UNVERIFIED (no measurement)</span>
            ) : (
              <span className={`font-bold ${activeMilestone.deltaMHa <= 1.6 ? 'text-emerald-400' : 'text-amber-400'}`}>
                Δ = {activeMilestone.deltaMHa.toFixed(2)} mHa {activeMilestone.deltaMHa <= 1.6 ? '(Accuracy Met)' : '(Exploring)'}
              </span>
            )}
          </div>

          {/* Progressive Disclosure Code Diff */}
          {showCodeDiff && activeMilestone.codeDiff && (
            <div className="pt-3 border-t border-white/[0.08] space-y-2">
              <span className="text-xs font-medium text-slate-200 flex items-center space-x-1.5">
                <FileCode className="w-3.5 h-3.5 text-cyan-400" />
                <span>Quantum Circuit Mutation</span>
              </span>
              <pre className="p-3 rounded-xl bg-black/80 border border-white/10 text-[11px] font-mono text-cyan-200 overflow-x-auto leading-relaxed">
                {activeMilestone.codeDiff}
              </pre>
            </div>
          )}
        </div>

        {/* Toggle Code Mutation Diff */}
        <button
          onClick={() => setShowCodeDiff(!showCodeDiff)}
          className="mt-3 text-xs text-slate-300 hover:text-white transition-colors flex items-center space-x-1 font-medium"
        >
          <span>{showCodeDiff ? 'Hide Circuit Code' : 'Inspect Circuit Code Mutation'}</span>
          {showCodeDiff ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
        </button>
      </div>

      {/* Bottom Cinematic Scrubber & Controls */}
      <div className="border-t border-white/[0.08] pt-6 space-y-4">
        {/* Play/Pause & Step Indicators */}
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <button
              onClick={() => setIsPlaying(!isPlaying)}
              className="w-9 h-9 rounded-full bg-white/10 hover:bg-white/20 active:scale-95 text-white flex items-center justify-center transition-all shadow-md focus:outline-none"
              title={isPlaying ? 'Pause Narrative' : 'Play Narrative'}
            >
              {isPlaying ? <Pause className="w-4 h-4" /> : <Play className="w-4 h-4 ml-0.5" />}
            </button>
            <button
              onClick={() => {
                setIsPlaying(false);
                setCurrentStepIndex(0);
              }}
              className="p-2 rounded-lg text-slate-300 hover:text-white hover:bg-white/10 transition-colors"
              title="Reset Timeline to Start"
            >
              <RotateCcw className="w-4 h-4" />
            </button>
          </div>

          <span className="text-xs font-mono text-slate-200 font-medium">
            Playback Scrubbing • Step {safeIndex + 1} of {milestones.length}
          </span>
        </div>

        {/* Interactive Timeline Track */}
        <div className="relative flex items-center h-4">
          <input 
            type="range"
            min={0}
            max={milestones.length - 1}
            step={1}
            value={safeIndex}
            onChange={(e) => {
              setIsPlaying(false);
              setCurrentStepIndex(parseInt(e.target.value));
            }}
            className="w-full h-2 bg-slate-700 rounded-lg appearance-none cursor-pointer accent-cyan-400 focus:outline-none"
          />
        </div>

        {/* Milestone Tick Labels (High-Contrast Tactile Cards) */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 pt-1 text-xs font-mono">
          {milestones.map((m, idx) => (
            <button
              key={idx}
              onClick={() => {
                setIsPlaying(false);
                setCurrentStepIndex(idx);
              }}
              title={m.title}
              className={`p-2 rounded-xl transition-all text-left flex items-center space-x-2 border ${
                idx === safeIndex
                  ? 'text-cyan-200 font-bold bg-cyan-950/70 border-cyan-500/60 shadow-[0_0_12px_rgba(6,182,212,0.25)]'
                  : 'text-slate-200 hover:text-white bg-white/[0.04] hover:bg-white/[0.08] border-white/10'
              }`}
            >
              <span className={`w-4 h-4 rounded-full flex items-center justify-center text-[10px] shrink-0 font-bold ${
                idx === safeIndex ? 'bg-cyan-400 text-black' : 'bg-white/15 text-slate-200'
              }`}>
                {m.step}
              </span>
              <span className="truncate">{m.title}</span>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
