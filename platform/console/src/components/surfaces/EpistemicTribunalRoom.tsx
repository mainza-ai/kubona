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
  targetSystem = '',
  sessionId = '',
  onCommitToLedger,
  onCampaignSynthesized,
}: TribunalRoomProps) {
  const effectiveTarget = targetSystem.trim() || 'Active target';
  const effectiveHypothesisId = hypothesisId.trim() || 'hyp-unassigned';
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
  const [topic, setTopic] = useState(`${effectiveTarget} ground state electronic structure`);

  // Sync topic when targetSystem changes
  const [prevTarget, setPrevTarget] = useState(effectiveTarget);
  if (prevTarget !== effectiveTarget) {
    setPrevTarget(effectiveTarget);
    setTopic(`${effectiveTarget} ground state electronic structure`);
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
        badge_color: 'border-[var(--color-info-edge)] text-[var(--color-info-ink)] bg-[var(--color-info-wash)]',
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
        badge_color: 'border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] bg-[var(--color-danger-wash)]',
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
        badge_color: 'border-[var(--color-warning-edge)] text-[var(--color-warning-ink)] bg-[var(--color-warning-wash)]',
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
        badge_color: 'border-[var(--color-success-edge)] text-[var(--color-success-ink)] bg-[var(--color-success-wash)]',
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
      const res = await runTribunal(sessionId, topic.trim() || `${effectiveTarget} ground state`);
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
      const state = await synthesizeCampaign(sessionId, topic.trim() || `${effectiveTarget} ground state`);
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
    : effectiveHypothesisId;

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
        hypothesis_id: effectiveHypothesisId,
        target_system: effectiveTarget,
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
          hypothesis_id: effectiveHypothesisId,
          target_system: effectiveTarget,
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
      <div className="bg-[var(--color-surface)] border border-[var(--color-border)] rounded-xl p-5 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2 mb-1">
            <span className="p-1.5 rounded-lg bg-[var(--color-soft-indigo-wash)] text-[var(--color-accent-ink)] border border-[var(--color-soft-indigo-edge)]">
              <Scale className="w-5 h-5" />
            </span>
            <h2 className="text-xl font-bold text-[var(--color-text)] tracking-wide">
              Epistemic Tribunal Room & Swarm Deliberation
            </h2>
            <span className={`text-[12px] font-mono uppercase px-2 py-0.5 rounded-full border ${
              isSimulating
                ? 'bg-[var(--color-info-wash)] border-[var(--color-info-edge)] text-[var(--color-info-ink)] animate-pulse'
                : turns
                  ? 'bg-[var(--color-success-wash)] border-[var(--color-success-edge)] text-[var(--color-success-ink)]'
                  : 'bg-[var(--color-tile)] border-[var(--color-border)] text-[var(--color-muted)]'
            }`}>
              {isSimulating ? 'Deliberating…' : turns ? 'Chamber Live' : 'Chamber Idle'}
            </span>
          </div>
          <p className="text-xs text-[var(--color-muted)]">
            Adversarial multi-agent deliberation chamber evaluating candidate discoveries under formal Lean 4 axioms,
            quantum advantage realism, and calibrated supermajority consensus.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={handleRunDeliberation}
            disabled={isSimulating}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-[var(--color-tile)] hover:bg-[var(--color-fill-hover)] text-xs text-[var(--color-text)] border border-[var(--color-border)] transition"
          >
            {isSimulating ? (
              <>
                <RotateCcw className="w-3.5 h-3.5 animate-spin text-[var(--color-info-ink)]" />
                <span>Deliberating...</span>
              </>
            ) : (
              <>
                <Play className="w-3.5 h-3.5 text-[var(--color-info-ink)]" />
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
                ? 'bg-[var(--color-success-wash)] text-[var(--color-success-ink)] border-[var(--color-success-edge)] cursor-default'
                : quorumPercent != null && quorumPercent >= 75.0
                ? 'bg-[var(--color-accent)] hover:bg-[var(--color-lit)] text-[var(--color-on-accent)] border-[var(--color-accent-edge)] shadow-sm'
                : 'bg-[var(--color-tile)] text-[var(--color-faint)] border-[var(--color-border)] cursor-not-allowed'
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
                ? 'bg-[var(--color-accent)] hover:bg-[var(--color-lit)] text-[var(--color-on-accent)] border-[var(--color-accent-edge)] shadow-sm'
                : 'bg-[var(--color-tile)] text-[var(--color-faint)] border-[var(--color-border)] cursor-not-allowed'
            }`}
          >
            <Sparkles className="w-4 h-4" />
            <span>{isSynthesizing ? 'Synthesizing…' : 'Synthesize Campaign'}</span>
          </button>
        </div>
      </div>
      {synthNotice && (
        <div className="rounded-xl border border-[var(--color-info-edge)] bg-[var(--color-info-wash)] px-4 py-2 text-xs text-[var(--color-info-ink)]" role="status">
          {synthNotice}
        </div>
      )}

      {/* 5-Phase Tribunal Protocol Progression */}
      <div className="bg-[var(--color-tile)] border border-[var(--color-border)] rounded-xl p-4">
        <div className="text-xs font-mono text-[var(--color-muted)] uppercase tracking-wider mb-3">
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
                    ? 'bg-[var(--color-soft-indigo-wash)] border-[var(--color-accent-edge)] shadow-sm'
                    : isDone
                    ? 'bg-[var(--color-surface)] border-[var(--color-success-edge)]'
                    : 'bg-[var(--color-surface)] border-[var(--color-border)] opacity-60'
                }`}
              >
                <div className="flex items-center justify-between mb-1">
                  <span className="text-[12px] font-mono text-[var(--color-muted)]">Phase {p.num}</span>
                  {isDone ? (
                    <CheckCircle2 className="w-3.5 h-3.5 text-[var(--color-success-ink)]" />
                  ) : isCurrent ? (
                    <Sparkles className="w-3.5 h-3.5 text-[var(--color-accent-ink)] animate-pulse" />
                  ) : (
                    <span className="w-2 h-2 rounded-full bg-[var(--color-faint)]" />
                  )}
                </div>
                <div className="text-xs font-bold text-[var(--color-text)] mb-0.5">{p.label}</div>
                <div className="text-[12px] text-[var(--color-muted)] leading-tight">{p.desc}</div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Active Candidate Hypothesis Card */}
      <div className="bg-[var(--color-surface)] border border-[var(--color-border)] rounded-xl p-4">
        <div className="flex flex-wrap items-center justify-between gap-2 mb-2">
          <div className="flex items-center gap-2">
            <span className="text-xs font-mono px-2 py-0.5 rounded bg-[var(--color-info-wash)] text-[var(--color-info-ink)] border border-[var(--color-info-edge)]">
              {effectiveHypothesisId}
            </span>
            <span className="text-xs font-semibold text-[var(--color-text)]">Target: {effectiveTarget}</span>
          </div>
          <div className="flex items-center gap-3 text-xs text-[var(--color-muted)] font-mono">
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
            className="flex-1 min-w-[220px] px-3 py-1.5 rounded-lg bg-[var(--color-field)] border border-[var(--color-border)] text-xs text-[var(--color-text)] font-mono focus:outline-none focus:border-[var(--color-accent-edge)]"
          />
        </div>
        {delibError && (
          <div className="p-3 bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] rounded-xl flex items-center gap-2 mb-2">
            <AlertTriangle className="w-4 h-4 text-[var(--color-danger-ink)] shrink-0" />
            <span className="text-xs text-[var(--color-danger-ink)]"><strong>Deliberation failed:</strong> {delibError} Nothing was recorded.</span>
          </div>
        )}

        {turns ? (
          <div className="bg-[var(--color-tile)] border border-[var(--color-border)] rounded-lg p-3 font-mono text-xs text-[var(--color-label-secondary)]">
            <div className="text-[var(--color-muted)] mb-1">Deliberation record ({turns.length} turns):</div>
            {turns.map((t, i) => (
              <div key={i} className="py-1 border-t border-[var(--color-border)] first:border-0">
                <span className="text-[var(--color-info-ink)]">[{t.role}]</span>{' '}
                <span className="text-[var(--color-label-secondary)]">{t.content.slice(0, 220)}{t.content.length > 220 ? '…' : ''}</span>
              </div>
            ))}
          </div>
        ) : (
          <div className="bg-[var(--color-tile)] border border-[var(--color-border)] rounded-lg p-3 font-mono text-xs text-[var(--color-faint)]">
            No deliberation record — convene the tribunal to fill this chamber.
          </div>
        )}
      </div>

      {/* Quorum Consensus Gauge */}
      <div className="bg-[var(--color-surface)] border border-[var(--color-border)] rounded-xl p-4 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div className="space-y-1">
          <div className="text-xs font-mono text-[var(--color-muted)] uppercase tracking-wider">
            Supermajority Quorum Consensus
          </div>
          <div className="flex items-center gap-3">
            <span className="text-2xl font-bold font-mono text-[var(--color-success-ink)]">{quorumPercent != null ? `${quorumPercent.toFixed(1)}%` : '—'}</span>
            <span className="text-xs text-[var(--color-muted)]">
              (Supermajority threshold: 75.0% required to commit discovery to Merkle ledger)
            </span>
          </div>
        </div>

        <div
          className="w-full md:w-64 bg-[var(--color-tile)] rounded-full h-3 overflow-hidden border border-[var(--color-border)]"
          role="progressbar"
          aria-label="Tribunal quorum consensus"
          aria-valuenow={quorumPercent != null ? Math.round(quorumPercent * 10) / 10 : 0}
          aria-valuemin={0}
          aria-valuemax={100}
        >
          <div
            className="bg-[var(--color-success)] h-full rounded-full transition-all duration-500"
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
              Seal hypothesis <strong className="font-mono text-[var(--color-text)]">{effectiveHypothesisId}</strong> for target{' '}
              <strong className="text-[var(--color-text)]">{effectiveTarget}</strong> at quorum{' '}
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
        <div className="p-3.5 bg-[var(--color-success-wash)] border border-[var(--color-success-edge)] rounded-xl flex items-center justify-between gap-2">
          <div className="flex items-center gap-2 text-xs text-[var(--color-success-ink)]">
            <CheckCircle2 className="w-4 h-4 text-[var(--color-success-ink)] shrink-0" />
            <span>
              <strong>Discovery Committed to Merkle Ledger:</strong> Receipt SHA256:{' '}
              <code className="font-mono text-[var(--color-text)]">{committedReceipt}</code>
            </span>
          </div>
          <span className="text-[12px] font-mono uppercase px-2 py-0.5 rounded bg-[var(--color-surface)] text-[var(--color-success-ink)] border border-[var(--color-success-edge)]">
            Quorum-sealed · server receipt
          </span>
        </div>
      )}

      {/* Commit Failure Notice (only when the gate dialog is closed —
          while open, the refusal lives in the dialog error slot) */}
      {commitError && !committedReceipt && !commitOpen && (
        <div className="p-3.5 bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] rounded-xl flex items-center gap-2">
          <AlertTriangle className="w-4 h-4 text-[var(--color-danger-ink)] shrink-0" />
          <span className="text-xs text-[var(--color-danger-ink)]">
            <strong>Ledger commit failed:</strong> {commitError} Nothing was recorded.
          </span>
        </div>
      )}

      {/* The Deliberation Grid: chamber seats, live turns only */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {filledSeats.map((item) => (
          <div
            key={item.persona.id}
            className="bg-[var(--color-surface)] border border-[var(--color-border)] hover:border-[var(--color-border-strong)] transition rounded-xl p-4 flex flex-col justify-between"
          >
            <div>
              {/* Persona Header */}
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-2">
                  <span className="text-xl">{item.persona.avatar_emoji}</span>
                  <div>
                    <h3 className="text-xs font-bold text-[var(--color-text)] leading-tight">{item.persona.name}</h3>
                    <span className="text-[12px] font-mono text-[var(--color-faint)]">{item.persona.role}</span>
                  </div>
                </div>
                <span
                  className={`text-[12px] font-mono uppercase px-2 py-0.5 rounded border ${
                    item.vote === 'corroborated'
                      ? 'border-[var(--color-success-edge)] text-[var(--color-success-ink)] bg-[var(--color-success-wash)]'
                      : item.vote === 'pending'
                        ? 'border-[var(--color-border)] text-[var(--color-muted)] bg-[var(--color-tile)]'
                        : 'border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] bg-[var(--color-danger-wash)]'
                  }`}
                >
                  {item.vote}
                </span>
              </div>

              {/* Seat statement: live turn content, or an honest empty seat */}
              <div className="p-2.5 rounded-lg bg-[var(--color-tile)] border border-[var(--color-border)] text-xs text-[var(--color-label-secondary)] leading-relaxed my-2">
                {item.statement ?? <span className="text-[var(--color-faint)] font-mono">— awaiting deliberation</span>}
              </div>
            </div>

            {/* Seat footer */}
            <div className="pt-2 border-t border-[var(--color-border)] flex items-center justify-between text-[12px] font-mono text-[var(--color-muted)]">
              <div className="flex items-center gap-1">
                <span>{item.statement ? 'live turn' : 'seat empty'}</span>
              </div>
              <div className="text-[var(--color-faint)] truncate max-w-[120px]">{item.persona.id}</div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
