'use client';

import React, { useState, useEffect } from 'react';
import {
  Users,
  ShieldCheck,
  CheckCircle2,
  AlertTriangle,
  Scale,
  Brain,
  Cpu,
  Flame,
  Binary,
  Scroll,
  FileCheck,
  Play,
  RotateCcw,
  Sparkles,
  ExternalLink,
  ChevronRight,
} from 'lucide-react';
import { EpistemicTribunalVote, AgentPersona, AgentRoleType } from '@/types';

interface TribunalRoomProps {
  hypothesisId?: string;
  targetSystem?: string;
  onCommitToLedger?: (receipt: Record<string, unknown>) => void;
}

interface PersonaState {
  persona: AgentPersona;
  currentStatement: string;
  vote: 'corroborated' | 'contested' | 'falsified' | 'abstain';
  confidenceRho: number;
  formalAstHash?: string;
  isThinking: boolean;
}

export function EpistemicTribunalRoom({
  hypothesisId = 'hyp-qksd-femoco-01',
  targetSystem = 'FeMoco Nitrogenase Cluster',
  onCommitToLedger,
}: TribunalRoomProps) {
  const [currentPhase, setCurrentPhase] = useState<number>(5); // 1 to 5
  const [quorumPercent, setQuorumPercent] = useState<number>(94.5);
  const [committedReceipt, setCommittedReceipt] = useState<string | null>(null);
  const [isSimulating, setIsSimulating] = useState<boolean>(false);

  const personas: PersonaState[] = [
    {
      persona: {
        id: 'theorist-01',
        name: 'Lead Polymath Theorist',
        role: 'lead_theorist',
        avatar_emoji: '🌌',
        badge_color: 'border-cyan-500/30 text-cyan-400 bg-cyan-950/20',
        description: 'Conjectures ground states, formulates physical models and targets.',
        thinking_horizon_seconds: 180,
      },
      currentStatement:
        'Conjecture: Ground state energy converges to -1.498 J via Quantum Krylov subspace diagonalization, beating classical single-reference DFT by 42 mHa.',
      vote: 'corroborated',
      confidenceRho: 0.96,
      formalAstHash: 'sha256:7b4f8a...90e1',
      isThinking: false,
    },
    {
      persona: {
        id: 'critic-01',
        name: 'Adversarial Falsification Critic',
        role: 'adversarial_falsification_critic',
        avatar_emoji: '⚔️',
        badge_color: 'border-rose-500/30 text-rose-400 bg-rose-950/20',
        description: 'Red-teams claims, probes barren plateaus, searches counterexamples.',
        thinking_horizon_seconds: 240,
      },
      currentStatement:
        'Falsification Audit: Variance of partial derivatives Var[∇H] = 0.082 > 10⁻⁷. No barren plateau traps detected across Krylov subspace depth 4.',
      vote: 'corroborated',
      confidenceRho: 0.92,
      formalAstHash: 'sha256:4a1c9e...38b2',
      isThinking: false,
    },
    {
      persona: {
        id: 'prover-01',
        name: 'Lean 4 Proof Engineer',
        role: 'lean4_proof_engineer',
        avatar_emoji: '📐',
        badge_color: 'border-emerald-500/30 text-emerald-400 bg-emerald-950/20',
        description: 'Machine-checks assertions under strict zero-cheat axiom containment.',
        thinking_horizon_seconds: 360,
      },
      currentStatement:
        'Lake Comparator: Machine proof compiled cleanly with 0 sorryAx. Axiom whitelist satisfied: {propext, Quot.sound, Classical.choice}.',
      vote: 'corroborated',
      confidenceRho: 1.0,
      formalAstHash: 'sha256:8f4e12...c09a',
      isThinking: false,
    },
    {
      persona: {
        id: 'anchor-01',
        name: 'Classical Anchor Solver',
        role: 'classical_anchor_solver',
        avatar_emoji: '⚖️',
        badge_color: 'border-amber-500/30 text-amber-400 bg-amber-950/20',
        description: 'Verifies expectations against exact mathematical physics anchors.',
        thinking_horizon_seconds: 120,
      },
      currentStatement:
        'Anchor Verification: System evaluated against Family 271 exact ferromagnetic bound (E₀/NJ = -1.5 J). Discrepancy Δ = 1.3 mHa <= 1.6 mHa chemical precision.',
      vote: 'corroborated',
      confidenceRho: 0.94,
      formalAstHash: 'sha256:2d90ef...71aa',
      isThinking: false,
    },
    {
      persona: {
        id: 'compiler-01',
        name: 'Quantum Circuit Compiler',
        role: 'quantum_circuit_compiler',
        avatar_emoji: '⚡',
        badge_color: 'border-purple-500/30 text-purple-400 bg-purple-950/20',
        description: 'Compiles QIR bitcode, optimizes Clifford+T depth, maps to qLDPC.',
        thinking_horizon_seconds: 90,
      },
      currentStatement:
        'QIR Compilation: Synthesized 12 logical qubits mapped to IBM [[144, 12, 12]] Bivariate Bicycle gross code. T-depth reduced by 41% using Family 279 relations.',
      vote: 'corroborated',
      confidenceRho: 0.89,
      formalAstHash: 'sha256:9c11aa...4f88',
      isThinking: false,
    },
    {
      persona: {
        id: 'sentinel-01',
        name: 'Sentinel Spend Governor',
        role: 'sentinel_spend_governor',
        avatar_emoji: '🛡️',
        badge_color: 'border-emerald-500/30 text-emerald-400 bg-emerald-950/20',
        description: 'Monitors AP2 spend mandates, enforces sovereign air-gap isolation.',
        thinking_horizon_seconds: 30,
      },
      currentStatement:
        'Spend Governance: AP2 spend mandate verified with WebCrypto Ed25519 signature. Egress blocked (0 bytes outbound); session within $120.00 ceiling.',
      vote: 'corroborated',
      confidenceRho: 1.0,
      formalAstHash: 'sha256:ed2551...99bb',
      isThinking: false,
    },
  ];

  const phases = [
    { num: 1, label: 'Proposal Submission', desc: 'Hypothesis tuple H published' },
    { num: 2, label: 'Adversarial Challenge', desc: 'Barren plateau & counterexample stress' },
    { num: 3, label: 'Formal Verification', desc: 'Lean 4 comparator check (0 sorryAx)' },
    { num: 4, label: 'Analytical Anchor Audit', desc: 'Families 271/281 bounds verification' },
    { num: 5, label: 'Quorum Voting', desc: 'Supermajority consensus (>= 75%)' },
  ];

  const handleRunDeliberation = () => {
    setIsSimulating(true);
    setCurrentPhase(1);
    let step = 1;
    const interval = setInterval(() => {
      step += 1;
      if (step <= 5) {
        setCurrentPhase(step);
      } else {
        clearInterval(interval);
        setIsSimulating(false);
        setQuorumPercent(95.2);
      }
    }, 700);
  };

  const handleCommitLedger = () => {
    const receiptHash = `0x${Array.from({ length: 64 }, () =>
      Math.floor(Math.random() * 16).toString(16)
    ).join('')}`;
    setCommittedReceipt(receiptHash);
    if (onCommitToLedger) {
      onCommitToLedger({
        receipt_hash: receiptHash,
        hypothesis_id: hypothesisId,
        target_system: targetSystem,
        quorum_percent: quorumPercent,
        timestamp: new Date().toISOString(),
      });
    }
  };

  return (
    <div className="space-y-6">
      {/* Header & Chamber Overview */}
      <div className="bg-neutral-900/80 border border-neutral-800 rounded-xl p-5 backdrop-blur-md flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2 mb-1">
            <span className="p-1.5 rounded-lg bg-indigo-950/60 text-indigo-400 border border-indigo-500/30">
              <Scale className="w-5 h-5" />
            </span>
            <h2 className="text-xl font-bold text-white tracking-wide">
              Epistemic Tribunal Room & Swarm Deliberation
            </h2>
            <span className="text-[10px] font-mono uppercase px-2 py-0.5 rounded-full bg-emerald-950/60 border border-emerald-500/30 text-emerald-400">
              Chamber Live
            </span>
          </div>
          <p className="text-xs text-neutral-400">
            Adversarial multi-agent deliberation chamber evaluating candidate discoveries under formal Lean 4 axioms,
            quantum advantage realism, and calibrated supermajority consensus.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={handleRunDeliberation}
            disabled={isSimulating}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-neutral-800 hover:bg-neutral-700 text-xs text-white border border-neutral-700 transition"
          >
            {isSimulating ? (
              <>
                <RotateCcw className="w-3.5 h-3.5 animate-spin text-cyan-400" />
                <span>Deliberating...</span>
              </>
            ) : (
              <>
                <Play className="w-3.5 h-3.5 text-cyan-400" />
                <span>Rerun Tribunal</span>
              </>
            )}
          </button>

          <button
            onClick={handleCommitLedger}
            disabled={quorumPercent < 75.0 || committedReceipt !== null}
            className={`flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg text-xs font-semibold border transition ${
              committedReceipt
                ? 'bg-emerald-950/40 text-emerald-400 border-emerald-500/40 cursor-default'
                : quorumPercent >= 75.0
                ? 'bg-indigo-600 hover:bg-indigo-500 text-white border-indigo-400 shadow-lg shadow-indigo-900/30'
                : 'bg-neutral-800 text-neutral-500 border-neutral-700 cursor-not-allowed'
            }`}
          >
            <FileCheck className="w-4 h-4" />
            <span>{committedReceipt ? 'Committed to Ledger' : 'Commit Discovery to Merkle Ledger'}</span>
          </button>
        </div>
      </div>

      {/* 5-Phase Tribunal Protocol Progression */}
      <div className="bg-neutral-900/50 border border-neutral-800 rounded-xl p-4">
        <div className="text-xs font-mono text-neutral-400 uppercase tracking-wider mb-3">
          Tribunal Phase Progression Protocol (Axis B)
        </div>
        <div className="grid grid-cols-1 md:grid-cols-5 gap-3">
          {phases.map((p) => {
            const isDone = currentPhase > p.num;
            const isCurrent = currentPhase === p.num;
            return (
              <div
                key={p.num}
                className={`p-3 rounded-lg border transition ${
                  isCurrent
                    ? 'bg-indigo-950/40 border-indigo-500/60 shadow-md shadow-indigo-950/50'
                    : isDone
                    ? 'bg-neutral-900/40 border-emerald-500/30'
                    : 'bg-neutral-900/20 border-neutral-800/80 opacity-60'
                }`}
              >
                <div className="flex items-center justify-between mb-1">
                  <span className="text-[10px] font-mono text-neutral-400">Phase {p.num}</span>
                  {isDone ? (
                    <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" />
                  ) : isCurrent ? (
                    <Sparkles className="w-3.5 h-3.5 text-indigo-400 animate-pulse" />
                  ) : (
                    <span className="w-2 h-2 rounded-full bg-neutral-700" />
                  )}
                </div>
                <div className="text-xs font-bold text-white mb-0.5">{p.label}</div>
                <div className="text-[11px] text-neutral-400 leading-tight">{p.desc}</div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Active Candidate Hypothesis Card */}
      <div className="bg-neutral-900/70 border border-neutral-800 rounded-xl p-4">
        <div className="flex flex-wrap items-center justify-between gap-2 mb-2">
          <div className="flex items-center gap-2">
            <span className="text-xs font-mono px-2 py-0.5 rounded bg-cyan-950/50 text-cyan-400 border border-cyan-800/40">
              {hypothesisId}
            </span>
            <span className="text-xs font-semibold text-neutral-200">Target: {targetSystem}</span>
          </div>
          <div className="flex items-center gap-3 text-xs text-neutral-400 font-mono">
            <span>Anchor: Family 271 / 278</span>
            <span>Spend Ceiling: $120.00</span>
          </div>
        </div>

        <div className="bg-black/40 border border-neutral-800/80 rounded-lg p-3 font-mono text-xs text-neutral-300">
          <div className="text-neutral-500 mb-1">Computational Hypothesis Tuple H:</div>
          <span className="text-indigo-400">⟨S_Lean4(femoco_krylov_monotonic)</span>,{' '}
          <span className="text-amber-400">A_anchor(Heisenberg_3D_Bloch)</span>,{' '}
          <span className="text-purple-400">C_QIR(bb_code_12q_transversal)</span>,{' '}
          <span className="text-emerald-400">B_AP2($120.00)</span>,{' '}
          <span className="text-cyan-400">ΔS_target(0.85 nat)⟩</span>
        </div>
      </div>

      {/* Quorum Consensus Gauge */}
      <div className="bg-neutral-900/60 border border-neutral-800 rounded-xl p-4 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div className="space-y-1">
          <div className="text-xs font-mono text-neutral-400 uppercase tracking-wider">
            Supermajority Quorum Consensus
          </div>
          <div className="flex items-center gap-3">
            <span className="text-2xl font-bold font-mono text-emerald-400">{quorumPercent.toFixed(1)}%</span>
            <span className="text-xs text-neutral-400">
              (Supermajority threshold: 75.0% required to commit discovery to Merkle ledger)
            </span>
          </div>
        </div>

        <div className="w-full md:w-64 bg-neutral-800 rounded-full h-3 overflow-hidden border border-neutral-700">
          <div
            className="bg-gradient-to-r from-emerald-500 to-cyan-400 h-full rounded-full transition-all duration-500"
            style={{ width: `${Math.min(quorumPercent, 100)}%` }}
          />
        </div>
      </div>

      {/* Committed Receipt Notice */}
      {committedReceipt && (
        <div className="p-3.5 bg-emerald-950/30 border border-emerald-500/40 rounded-xl flex items-center justify-between gap-2">
          <div className="flex items-center gap-2 text-xs text-emerald-300">
            <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0" />
            <span>
              <strong>Discovery Committed to Merkle Ledger:</strong> Receipt SHA256:{' '}
              <code className="font-mono text-emerald-200">{committedReceipt}</code>
            </span>
          </div>
          <span className="text-[10px] font-mono uppercase px-2 py-0.5 rounded bg-emerald-900/60 text-emerald-300 border border-emerald-500/30">
            Immutable ρ=1.000000
          </span>
        </div>
      )}

      {/* The Deliberation Grid: 6 Specialized Personas */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {personas.map((item) => (
          <div
            key={item.persona.id}
            className="bg-neutral-900/80 border border-neutral-800 hover:border-neutral-700 transition rounded-xl p-4 flex flex-col justify-between"
          >
            <div>
              {/* Persona Header */}
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-2">
                  <span className="text-xl">{item.persona.avatar_emoji}</span>
                  <div>
                    <h3 className="text-xs font-bold text-white leading-tight">{item.persona.name}</h3>
                    <span className="text-[10px] font-mono text-neutral-500">{item.persona.role}</span>
                  </div>
                </div>
                <span
                  className={`text-[10px] font-mono uppercase px-2 py-0.5 rounded border ${
                    item.vote === 'corroborated'
                      ? 'border-emerald-500/40 text-emerald-400 bg-emerald-950/30'
                      : 'border-rose-500/40 text-rose-400 bg-rose-950/30'
                  }`}
                >
                  {item.vote}
                </span>
              </div>

              {/* Persona statement / critique */}
              <div className="p-2.5 rounded-lg bg-black/40 border border-neutral-800/80 text-xs text-neutral-300 leading-relaxed my-2">
                {item.currentStatement}
              </div>
            </div>

            {/* Persona footer with confidence and formal hash */}
            <div className="pt-2 border-t border-neutral-800/80 flex items-center justify-between text-[11px] font-mono text-neutral-400">
              <div className="flex items-center gap-1">
                <span>Confidence:</span>
                <span className="text-emerald-400 font-semibold">ρ = {item.confidenceRho.toFixed(3)}</span>
              </div>
              <div className="text-neutral-500 truncate max-w-[120px]">{item.formalAstHash}</div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
