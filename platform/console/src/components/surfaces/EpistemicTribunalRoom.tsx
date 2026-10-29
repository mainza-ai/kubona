'use client';

import React, { useState, useEffect } from 'react';
import {
  CheckCircle2,
  AlertTriangle,
  Scale,
  FileCheck,
  Play,
  RotateCcw,
  Sparkles,
} from 'lucide-react';
import { AgentPersona, CampaignState, TribunalTurn } from '@/types';
import { runTribunal, appendLedgerEvent, synthesizeCampaign, fetchMessages } from '@/lib/api';
import { classifyTribunalVerdict } from '@/lib/verdict';
import ConfirmDialog from '@/components/common/ConfirmDialog';

interface TribunalRoomProps {
  hypothesisId?: string;
  targetSystem?: string;
  sessionId?: string;
  campaign?: CampaignState | null;
  onCommitToLedger?: (receipt: Record<string, unknown>) => void;
  onCampaignSynthesized?: (campaign: CampaignState) => void;
}

interface PersonaState {
  persona: AgentPersona;
  // Live turn content once a real deliberation lands; null = seat empty.
  statement: string | null;
  vote: 'corroborated' | 'contested' | 'falsified' | 'abstain' | 'pending';
  // Substrings of TribunalTurn.role that fill this seat.
  matchKeys: string[];
}

export function EpistemicTribunalRoom({
  hypothesisId = 'hyp-qksd-01',
  targetSystem = 'Lithium Hydride (LiH)',
  sessionId = '',
  onCommitToLedger,
  onCampaignSynthesized,
}: TribunalRoomProps) {
  const [currentPhase, setCurrentPhase] = useState<number>(0); // 0 = not started; 5 = consensus recorded
  const [quorumPercent, setQuorumPercent] = useState<number | null>(null);
  const [committedReceipt, setCommittedReceipt] = useState<string | null>(null);
  const [isSimulating, setIsSimulating] = useState<boolean>(false);
  const [isCommitting, setIsCommitting] = useState<boolean>(false);
  const [commitError, setCommitError] = useState<string | null>(null);
  const [delibError, setDelibError] = useState<string | null>(null);
  const [isSynthesizing, setIsSynthesizing] = useState<boolean>(false);
  const [synthNotice, setSynthNotice] = useState<string | null>(null);
  const [turns, setTurns] = useState<TribunalTurn[] | null>(null);
  // Wave-2 typed gate: the Merkle-ledger commit is single-click no more.
  // The outer button keeps the exact quorum gating below; it only stages
  // this dialog, which requires typing the target/session short-id.
  const [commitOpen, setCommitOpen] = useState(false);
  const [topic, setTopic] = useState(`${targetSystem} ground state electronic structure`);

  // Sync topic when targetSystem changes
  const [prevTarget, setPrevTarget] = useState(targetSystem);
  if (prevTarget !== targetSystem) {
    setPrevTarget(targetSystem);
    setTopic(`${targetSystem} ground state electronic structure`);
  }

  // Auto-load existing deliberation turns for the active session
  useEffect(() => {
    if (!sessionId) return;
    let live = true;
    fetchMessages(sessionId)
      .then((msgs) => {
        if (!live || !msgs || msgs.length === 0) return;
        // 1. Check for assistant message with tribunal payload
        const tribunalMsg = [...msgs].reverse().find((m) => m.tribunal?.turns && m.tribunal.turns.length > 0);
        if (tribunalMsg?.tribunal?.turns) {
          setTurns(tribunalMsg.tribunal.turns);
          const corroborated = tribunalMsg.tribunal.turns.filter((t) =>
            classifyTribunalVerdict(t.verdict) === 'corroborated'
          ).length;
          setQuorumPercent((corroborated / tribunalMsg.tribunal.turns.length) * 100);
          setCurrentPhase(5);
          return;
        }
        // 2. Or check for individual specialist persona messages
        const personaRoles = ['ansatz-scout', 'disconfirmation-officer', 'device-auditor', 'epistemic-scribe'];
        const turnsFound: TribunalTurn[] = [];
        for (const m of msgs) {
          if (personaRoles.some((r) => m.role.toLowerCase().includes(r))) {
            turnsFound.push({
              turn_index: turnsFound.length + 1,
              role: m.role,
              name: m.role.replace(/-/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()),
              content: m.content,
              verdict: m.content.toLowerCase().includes('reject') || m.content.toLowerCase().includes('cannot be claimed') ? 'contested' : 'corroborated',
              timestamp: m.created_at,
            });
          }
        }
        if (turnsFound.length > 0) {
          setTurns(turnsFound);
          const corroborated = turnsFound.filter((t) =>
            classifyTribunalVerdict(t.verdict) === 'corroborated'
          ).length;
          setQuorumPercent((corroborated / turnsFound.length) * 100);
          setCurrentPhase(5);
        }
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [sessionId]);

  // Chamber seats aligned with Kubona's 4 Epistemic Collective Roles.
  // Statements, votes, and quorum come exclusively from live tribunal turns.
  const personas: PersonaState[] = [
    {
      persona: {
        id: 'scout-01',
        name: 'Ansatz Scout',
        role: 'ansatz_scout',
        avatar_emoji: '🌌',
        badge_color: 'border-cyan-500/30 text-cyan-400 bg-cyan-950/20',
        description: 'Proposes active space partitioning and low-depth variational quantum circuits.',
        thinking_horizon_seconds: 180,
      },
      statement: null,
      vote: 'pending',
      matchKeys: ['scout', 'ansatz', 'theorist'],
    },
    {
      persona: {
        id: 'critic-01',
        name: 'Disconfirmation Officer',
        role: 'disconfirmation_officer',
        avatar_emoji: '⚔️',
        badge_color: 'border-rose-500/30 text-rose-400 bg-rose-950/20',
        description: 'Red-teams claims against classical anchors, probes barren plateaus, enforces realism.',
        thinking_horizon_seconds: 240,
      },
      statement: null,
      vote: 'pending',
      matchKeys: ['disconfirmation', 'critic', 'falsif', 'officer'],
    },
    {
      persona: {
        id: 'auditor-01',
        name: 'Device Auditor',
        role: 'device_auditor',
        avatar_emoji: '⚖️',
        badge_color: 'border-amber-500/30 text-amber-400 bg-amber-950/20',
        description: 'Audits hardware noise models, canary probe fidelity, and shot-budget financial caps.',
        thinking_horizon_seconds: 120,
      },
      statement: null,
      vote: 'pending',
      matchKeys: ['device', 'auditor', 'hardware', 'anchor'],
    },
    {
      persona: {
        id: 'scribe-01',
        name: 'Epistemic Scribe',
        role: 'epistemic_scribe',
        avatar_emoji: '📐',
        badge_color: 'border-emerald-500/30 text-emerald-400 bg-emerald-950/20',
        description: 'Synthesizes supermajority consensus, collapses credibility wavefunctions, mints AP2 proposal.',
        thinking_horizon_seconds: 360,
      },
      statement: null,
      vote: 'pending',
      matchKeys: ['scribe', 'prover', 'lean', 'epistemic'],
    },
  ];

  const voteForVerdict = (verdict: string): PersonaState['vote'] => {
    const c = classifyTribunalVerdict(verdict);
    return c === 'abstain' ? 'abstain' : c;
  };

  // Fill seats from live turns only. No turn, no statement — ever.
  const filledSeats: PersonaState[] = personas.map((seat) => {
    const turn = turns?.find((t) =>
      seat.matchKeys.some((k) => `${t.role} ${t.name}`.toLowerCase().includes(k))
    );
    if (!turn) return seat;
    return { ...seat, statement: turn.content, vote: voteForVerdict(turn.verdict) };
  });

  const phases = [
    { num: 1, label: 'Proposal Submission', desc: 'Hypothesis tuple H published' },
    { num: 2, label: 'Adversarial Challenge', desc: 'Barren plateau & counterexample stress' },
    { num: 3, label: 'Formal Verification', desc: 'Lean 4 comparator check (0 sorryAx)' },
    { num: 4, label: 'Analytical Anchor Audit', desc: 'Families 271/281 bounds verification' },
    { num: 5, label: 'Quorum Voting', desc: 'Supermajority consensus (>= 75%)' },
  ];

  const handleRunDeliberation = async () => {
    if (isSimulating) return;
    if (!sessionId) {
      setDelibError('No active session: open or create a session before convening.');
      return;
    }
    setIsSimulating(true);
    setDelibError(null);
    setCommittedReceipt(null);
    try {
      // Real deliberation: one backend call, four live turns. Minutes on
      // local models — the button shows honest waiting, not fake phases.
      // runTribunal returns null on transport failure (distinct from an
      // empty-turns success), so the chamber can separate the two.
      const res = await runTribunal(sessionId, topic.trim() || `${targetSystem} ground state`);
      if (res == null) {
        setDelibError('Tribunal unreachable (transport failure) — nothing deliberated, nothing recorded.');
      } else if (res?.tribunal?.turns?.length) {
        setTurns(res.tribunal.turns);
        const corroborated = res.tribunal.turns.filter((t) =>
          classifyTribunalVerdict(t.verdict) === 'corroborated'
        ).length;
        setQuorumPercent((corroborated / res.tribunal.turns.length) * 100);
        setCurrentPhase(5);
      } else {
        setDelibError('Tribunal returned no turns — nothing deliberated, nothing recorded.');
      }
    } catch (err) {
      setDelibError(err instanceof Error ? err.message : 'Tribunal call failed.');
    } finally {
      setIsSimulating(false);
    }
  };

  // U2-b: formulate campaign state from the deliberation (same quorum
  // bar as commit — formulation follows consensus, never contest).
  // synthesizeCampaign returns null on refusal and throws only on
  // transport failure — the notice names which happened.
  const handleSynthesizeCampaign = async () => {
    if (!sessionId) {
      setSynthNotice('No active session: open or create a session before synthesizing.');
      return;
    }
    setIsSynthesizing(true);
    setSynthNotice(null);
    try {
      const state = await synthesizeCampaign(sessionId, topic.trim() || `${targetSystem} ground state`);
      if (!state) {
        setSynthNotice('Synthesis refused or failed — campaign unchanged, nothing recorded.');
        return;
      }
      setSynthNotice(`Campaign synthesized from deliberation — ledger-sealed server-side.`);
      if (onCampaignSynthesized) onCampaignSynthesized(state);
    } catch (err) {
      setSynthNotice(err instanceof Error ? err.message : 'Synthesis call failed — campaign unchanged.');
    } finally {
      setIsSynthesizing(false);
    }
  };

  // Short typed acknowledgement for the ledger commit: the leading 8 of
  // the bound session id, falling back to the hypothesis id when unbound.
  // 2dp money rule N/A here — no dollar figure is rendered on this gate.
  const commitRequireText = (sessionId || '').trim()
    ? sessionId.trim().slice(0, 8)
    : hypothesisId;

  const handleCommitLedger = async () => {
    if (!sessionId) {
      setCommitError('No active session: open or create a session before committing.');
      return;
    }
    setIsCommitting(true);
    setCommitError(null);
    try {
      // Authenticated bridge helper (authHeaders inside) — no raw fetch.
      // Surfaces the backend refusal body; throws only on transport.
      const res = await appendLedgerEvent(sessionId, 'tribunal_commit', {
        hypothesis_id: hypothesisId,
        target_system: targetSystem,
        quorum_percent: quorumPercent,
      });
      if (!res.ok) {
        // Stay open: the refusal renders in the dialog error slot.
        setCommitError(res.error || 'ledger refused the commit');
        return;
      }
      setCommittedReceipt(res.record_hash as string);
      setCommitOpen(false);
      setCommitError(null);
      if (onCommitToLedger) {
        onCommitToLedger({
          receipt_hash: res.record_hash,
          event_id: res.event_id,
          hypothesis_id: hypothesisId,
          target_system: targetSystem,
          quorum_percent: quorumPercent,
          timestamp: new Date().toISOString(),
        });
      }
    } catch (err) {
      setCommitError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsCommitting(false);
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
            <span className={`text-[10px] font-mono uppercase px-2 py-0.5 rounded-full border ${
              isSimulating
                ? 'bg-cyan-950/60 border-cyan-500/30 text-cyan-400 animate-pulse'
                : turns
                  ? 'bg-emerald-950/60 border-emerald-500/30 text-emerald-400'
                  : 'bg-white/[0.03] border-white/10 text-slate-500'
            }`}>
              {isSimulating ? 'Deliberating…' : turns ? 'Chamber Live' : 'Chamber Idle'}
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
            onClick={() => {
              if (!sessionId) {
                setCommitError('No active session: open or create a session before committing.');
                return;
              }
              setCommitError(null);
              setCommitOpen(true);
            }}
            disabled={quorumPercent == null || quorumPercent < 75.0 || committedReceipt !== null || isCommitting}
            className={`flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg text-xs font-semibold border transition ${
              committedReceipt
                ? 'bg-emerald-950/40 text-emerald-400 border-emerald-500/40 cursor-default'
                : quorumPercent != null && quorumPercent >= 75.0
                ? 'bg-indigo-600 hover:bg-indigo-500 text-white border-indigo-400 shadow-lg shadow-indigo-900/30'
                : 'bg-neutral-800 text-neutral-500 border-neutral-700 cursor-not-allowed'
            }`}
          >
            <FileCheck className="w-4 h-4" />
            <span>{committedReceipt ? 'Committed to Ledger' : isCommitting ? 'Committing…' : 'Commit Discovery to Merkle Ledger'}</span>
          </button>

          {/* U2-b: formulate campaign state from the deliberation. Same
              quorum bar as commit — formulation follows consensus. */}
          <button
            onClick={handleSynthesizeCampaign}
            disabled={quorumPercent == null || quorumPercent < 75.0 || isSynthesizing}
            title={quorumPercent == null || quorumPercent < 75.0 ? 'Needs supermajority consensus (≥ 75%) first' : 'Formulate campaign state from this deliberation'}
            className={`flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg text-xs font-semibold border transition ${
              quorumPercent != null && quorumPercent >= 75.0
                ? 'bg-cyan-600 hover:bg-cyan-500 text-white border-cyan-400 shadow-lg shadow-cyan-900/30'
                : 'bg-neutral-800 text-neutral-500 border-neutral-700 cursor-not-allowed'
            }`}
          >
            <Sparkles className="w-4 h-4" />
            <span>{isSynthesizing ? 'Synthesizing…' : 'Synthesize Campaign'}</span>
          </button>
        </div>
      </div>
      {synthNotice && (
        <div className="rounded-xl border border-cyan-500/30 bg-cyan-950/40 px-4 py-2 text-xs text-cyan-200" role="status">
          {synthNotice}
        </div>
      )}

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
            <span>{turns ? `${turns.length} live turns` : 'no deliberation yet'}</span>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2 mb-2">
          <input
            value={topic}
            onChange={(e) => setTopic(e.target.value)}
            aria-label="Deliberation topic"
            placeholder="Deliberation topic…"
            maxLength={200}
            className="flex-1 min-w-[220px] px-3 py-1.5 rounded-lg bg-black/40 border border-neutral-800 text-xs text-neutral-200 font-mono focus:outline-none focus:border-indigo-500"
          />
        </div>
        {delibError && (
          <div className="p-3 bg-rose-950/30 border border-rose-500/40 rounded-xl flex items-center gap-2 mb-2">
            <AlertTriangle className="w-4 h-4 text-rose-400 shrink-0" />
            <span className="text-xs text-rose-300"><strong>Deliberation failed:</strong> {delibError} Nothing was recorded.</span>
          </div>
        )}

        {turns ? (
          <div className="bg-black/40 border border-neutral-800/80 rounded-lg p-3 font-mono text-xs text-neutral-300">
            <div className="text-neutral-500 mb-1">Deliberation record ({turns.length} turns):</div>
            {turns.map((t, i) => (
              <div key={i} className="py-1 border-t border-neutral-800/60 first:border-0">
                <span className="text-cyan-400">[{t.role}]</span>{' '}
                <span className="text-neutral-300">{t.content.slice(0, 220)}{t.content.length > 220 ? '…' : ''}</span>
              </div>
            ))}
          </div>
        ) : (
          <div className="bg-black/40 border border-neutral-800/80 rounded-lg p-3 font-mono text-xs text-neutral-600">
            No deliberation record — convene the tribunal to fill this chamber.
          </div>
        )}
      </div>

      {/* Quorum Consensus Gauge */}
      <div className="bg-neutral-900/60 border border-neutral-800 rounded-xl p-4 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div className="space-y-1">
          <div className="text-xs font-mono text-neutral-400 uppercase tracking-wider">
            Supermajority Quorum Consensus
          </div>
          <div className="flex items-center gap-3">
            <span className="text-2xl font-bold font-mono text-emerald-400">{quorumPercent != null ? `${quorumPercent.toFixed(1)}%` : '—'}</span>
            <span className="text-xs text-neutral-400">
              (Supermajority threshold: 75.0% required to commit discovery to Merkle ledger)
            </span>
          </div>
        </div>

        <div
          className="w-full md:w-64 bg-neutral-800 rounded-full h-3 overflow-hidden border border-neutral-700"
          role="progressbar"
          aria-label="Tribunal quorum consensus"
          aria-valuenow={quorumPercent != null ? Math.round(quorumPercent * 10) / 10 : 0}
          aria-valuemin={0}
          aria-valuemax={100}
        >
          <div
            className="bg-gradient-to-r from-emerald-500 to-cyan-400 h-full rounded-full transition-all duration-500"
            style={{ width: `${quorumPercent != null ? Math.min(quorumPercent, 100) : 0}%` }}
          />
        </div>
      </div>

      {commitOpen && (
        <ConfirmDialog
          open
          title="Commit discovery to Merkle ledger?"
          body={
            <span>
              Seal hypothesis <strong className="font-mono text-white">{hypothesisId}</strong> for target{' '}
              <strong className="text-white">{targetSystem}</strong> at quorum{' '}
              {quorumPercent != null ? `${quorumPercent.toFixed(1)}%` : '—'} (session{' '}
              <code className="font-mono">{commitRequireText}</code>). Ledger-sealed and irreversible.
            </span>
          }
          requireText={commitRequireText}
          confirmLabel="Commit to ledger"
          danger
          busy={isCommitting}
          error={commitError}
          onConfirm={() => void handleCommitLedger()}
          onCancel={() => {
            if (isCommitting) return;
            setCommitOpen(false);
            setCommitError(null);
          }}
        />
      )}

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
            Quorum-sealed · server receipt
          </span>
        </div>
      )}

      {/* Commit Failure Notice (only when the gate dialog is closed —
          while open, the refusal lives in the dialog error slot) */}
      {commitError && !committedReceipt && !commitOpen && (
        <div className="p-3.5 bg-rose-950/30 border border-rose-500/40 rounded-xl flex items-center gap-2">
          <AlertTriangle className="w-4 h-4 text-rose-400 shrink-0" />
          <span className="text-xs text-rose-300">
            <strong>Ledger commit failed:</strong> {commitError} Nothing was recorded.
          </span>
        </div>
      )}

      {/* The Deliberation Grid: chamber seats, live turns only */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {filledSeats.map((item) => (
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
                      : item.vote === 'pending'
                        ? 'border-white/10 text-slate-500 bg-white/[0.03]'
                        : 'border-rose-500/40 text-rose-400 bg-rose-950/30'
                  }`}
                >
                  {item.vote}
                </span>
              </div>

              {/* Seat statement: live turn content, or an honest empty seat */}
              <div className="p-2.5 rounded-lg bg-black/40 border border-neutral-800/80 text-xs text-neutral-300 leading-relaxed my-2">
                {item.statement ?? <span className="text-neutral-600 font-mono">— awaiting deliberation</span>}
              </div>
            </div>

            {/* Seat footer */}
            <div className="pt-2 border-t border-neutral-800/80 flex items-center justify-between text-[11px] font-mono text-neutral-400">
              <div className="flex items-center gap-1">
                <span>{item.statement ? 'live turn' : 'seat empty'}</span>
              </div>
              <div className="text-neutral-500 truncate max-w-[120px]">{item.persona.id}</div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
