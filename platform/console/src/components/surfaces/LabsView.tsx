'use client';

import React, { useState, useEffect, useMemo } from 'react';
import { FlaskConical, ChevronDown, ChevronUp, AlertTriangle } from 'lucide-react';
import { Lab, Session } from '@/types';
import { fetchLabs, fetchSessions, authWhoami, wasRecentAuthFailure, AUTH_CHANGED_EVENT } from '@/lib/api';
import { fmtMoney } from '@/lib/guards';
import { LabOpsPanel } from '@/components/common/LabOpsPanel';
import { AuthPanel } from '@/components/common/AuthPanel';
import { CreateLabPanel } from '@/components/common/CreateLabPanel';
import { TenantGrantPanel } from '@/components/common/TenantGrantPanel';
import { KeyRecoveryPanel } from '@/components/common/KeyRecoveryPanel';
import { SessionRefundPanel } from '@/components/common/SessionRefundPanel';

function pct(spent: number, ceiling: number): number | null {
  if (!Number.isFinite(spent) || !Number.isFinite(ceiling) || !(ceiling > 0)) return null;
  return Math.min(100, Math.round((spent / ceiling) * 100));
}

export function LabsView() {
  const [labs, setLabs] = useState<Lab[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Auth signal for the empty state: an empty lab list after a 401 (or
  // with no authenticated session) is a refusal, not a genuine empty.
  // The wrapper returns [] for both, so this separate signal decides.
  const [authed, setAuthed] = useState<boolean | null>(null);

  // Refetch labs (U1-b: server owns tenant state — every accepted
  // ceiling/status/refund/admin-key mutation re-reads it).
  const reload = async () => {
    const l = await fetchLabs();
    setLabs(l);
  };

  useEffect(() => {
    let live = true;
    Promise.all([fetchLabs(), fetchSessions()])
      .then(([l, s]) => {
        if (!live) return;
        setLabs(l);
        setSessions(s);
        setLoaded(true);
      })
      .catch((e) => {
        if (!live) return;
        setError(e instanceof Error ? e.message : String(e));
        setLoaded(true);
      });
    // Auth check never blocks or fails the lab load — it only labels an
    // empty result as refused vs genuinely empty.
    authWhoami()
      .then((me) => {
        if (live) setAuthed(me?.authenticated === true);
      })
      .catch(() => {
        if (live) setAuthed(false);
      });
    return () => {
      live = false;
    };
  }, []);

  useEffect(() => {
    if (typeof window === 'undefined' || typeof AUTH_CHANGED_EVENT !== 'string') return;
    const onAuthChanged = () => {
      authWhoami()
        .then((me) => setAuthed(me?.authenticated === true))
        .catch(() => setAuthed(false));
      void reload();
    };
    window.addEventListener(AUTH_CHANGED_EVENT, onAuthChanged);
    return () => window.removeEventListener(AUTH_CHANGED_EVENT, onAuthChanged);
  }, []);

  const byLab = useMemo(() => {
    const m: Record<string, Session[]> = {};
    for (const s of sessions) {
      const key = s.tenant_id || 'default';
      (m[key] ??= []).push(s);
    }
    return m;
  }, [sessions]);

  if (!loaded) {
    // Wave-3: honest first-load skeleton for the lab list (gated on the
    // existing loaded flag — refused-vs-empty logic below is untouched).
    return (
      <div className="space-y-2.5">
        <div className="text-[var(--color-muted)] text-sm">Loading laboratories…</div>
        <div data-testid="labs-skeleton" className="space-y-2" aria-hidden="true">
          <div className="h-16 rounded-2xl bg-[var(--color-tile)] border border-[var(--color-border)] animate-pulse" />
          <div className="h-16 rounded-2xl bg-[var(--color-tile)] border border-[var(--color-border)] animate-pulse" />
          <div className="h-16 rounded-2xl bg-[var(--color-tile)] border border-[var(--color-border)] animate-pulse" />
        </div>
      </div>
    );
  }
  if (error) {
    return (
      <div className="p-3 rounded-xl bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-xs">
        Laboratories unavailable — nothing loaded ({error.slice(0, 200)}).
      </div>
    );
  }
  if (labs.length === 0) {
    // Refused (401 / no session) reads as a login prompt anchored to the
    // Operator session panel — never as a fake "no laboratories" empty.
    // wasRecentAuthFailure covers a 401 that raced this render; the
    // whoami result covers a steady logged-out state.
    if (wasRecentAuthFailure() || authed === false) {
      return (
        <div className="space-y-3">
          <div className="p-3 rounded-xl bg-[var(--color-warning-wash)] border border-[var(--color-warning-edge)] text-[var(--color-warning-ink)] text-xs">
            Sign in to manage laboratories. Research campaigns are under Campaigns.{' '}
            <a href="#operator-session" className="underline hover:opacity-80">
              Sign in below
            </a>
            .
          </div>
          <div id="operator-session">
            <AuthPanel labs={[]} />
          </div>
          <CreateLabPanel onCreated={reload} />
          {authed === true && (
            <details className="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
              <summary className="cursor-pointer text-xs font-semibold text-[var(--color-text)]">Advanced lab administration</summary>
              <div className="mt-3 space-y-3">
                <TenantGrantPanel labs={labs} />
                <KeyRecoveryPanel labs={labs} />
                <SessionRefundPanel labs={labs} sessions={sessions} />
              </div>
            </details>
          )}
        </div>
      );
    }
    return (
      <div className="space-y-3">
        <div className="text-[var(--color-muted)] text-sm">No laboratories registered.</div>
        <CreateLabPanel onCreated={reload} />
      </div>
    );
  }

  if (labs.length > 0 && authed !== true) {
    return (
      <div className="space-y-3">
        <p className="text-[12px] text-[var(--color-muted)]">
          Laboratories are administered by a lab admin. Sign in to manage this lab; research campaigns remain under Campaigns.
        </p>
        <AuthPanel labs={labs} />
        <CreateLabPanel onCreated={reload} />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <p className="text-[12px] text-[var(--color-muted)]">
        Laboratories are for multi-lab administration. Research campaigns live under Campaigns.
      </p>
      <AuthPanel labs={labs} />
      <details className="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
        <summary className="cursor-pointer text-xs font-semibold text-[var(--color-text)]">Advanced lab administration</summary>
        <div className="mt-3"><CreateLabPanel onCreated={reload} /></div>
      </details>
      {labs.map((lab) => {
        const spent = typeof lab.spent_usd === 'number' && Number.isFinite(lab.spent_usd) ? lab.spent_usd : null;
        const ceiling = typeof lab.spend_ceiling_usd === 'number' && Number.isFinite(lab.spend_ceiling_usd) ? lab.spend_ceiling_usd : null;
        const unbounded = ceiling == null || !(ceiling > 0);
        const used = !unbounded && spent != null && ceiling != null ? pct(spent, ceiling) : null;
        const hot = used != null && used >= 90;
        const labSessions = byLab[lab.id] || [];
        const expanded = !!open[lab.id];
        return (
          <div
            key={lab.id}
            className="rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] p-4"
          >
            <button
              className="w-full flex items-center justify-between text-left"
              onClick={() => setOpen((o) => ({ ...o, [lab.id]: !o[lab.id] }))}
              aria-expanded={expanded}
            >
              <span className="flex items-center space-x-2">
                <FlaskConical className="w-4 h-4 text-[var(--color-info-ink)]" />
                <span className="text-[var(--color-text)] font-medium text-sm">
                  {lab.display_name || lab.id}
                </span>
                <span className="text-[12px] font-mono px-1.5 py-0.5 rounded border bg-[var(--color-tile)] text-[var(--color-label-secondary)] border-[var(--color-border)]">
                  {lab.autonomy_level}
                </span>
                {lab.status !== 'active' && (
                  <span className="text-[12px] font-mono px-1.5 py-0.5 rounded border bg-[var(--color-warning-wash)] text-[var(--color-warning-ink)] border-[var(--color-warning-edge)]">
                    {lab.status}
                  </span>
                )}
                {unbounded && (
                  <span className="text-[12px] font-mono px-1.5 py-0.5 rounded border bg-[var(--color-danger-wash)] text-[var(--color-danger-ink)] border-[var(--color-danger-edge)]">
                    NO-CEILING · UNBOUNDED
                  </span>
                )}
              </span>
              <span className="flex items-center space-x-2 text-xs text-[var(--color-label-secondary)]">
                {unbounded ? (
                  <span>{spent != null ? fmtMoney(spent) : '—'} / UNBOUNDED (no ceiling)</span>
                ) : (
                  <span>{`${spent != null ? fmtMoney(spent) : '—'} / ${ceiling != null ? fmtMoney(ceiling) : '—'} (${used ?? '—'}%)`}</span>
                )}
                {hot && <AlertTriangle className="w-3.5 h-3.5 text-[var(--color-warning-ink)]" />}
                {expanded ? (
                  <ChevronUp className="w-4 h-4" />
                ) : (
                  <ChevronDown className="w-4 h-4" />
                )}
              </span>
            </button>
            <div
              className="mt-2 h-1.5 rounded-full bg-[var(--color-tile)]"
              role="progressbar"
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={unbounded ? 100 : (used ?? 0)}
              aria-valuetext={unbounded ? 'unbounded (no ceiling)' : `${used ?? '—'}% of ceiling`}
              aria-label={`Spend for ${lab.display_name || lab.id}`}
            >
              {unbounded ? (
                <div className="h-1.5 rounded-full bg-[var(--color-danger)]" style={{ width: '100%' }} title="No ceiling — unbounded spend alarm" />
              ) : (
                <div
                  className={`h-1.5 rounded-full ${hot ? 'bg-[var(--color-warning)]' : 'bg-[var(--color-accent)]'}`}
                  style={{ width: `${used ?? 0}%` }}
                />
              )}
            </div>
            {expanded && (
              <div className="mt-3 space-y-1.5">
                {/* U1-b: operator writes (ceiling/suspend/refund/admin) —
                    bearer + admin envelope, refusals verbatim. */}
                <LabOpsPanel lab={lab} onChanged={reload} />
                {labSessions.length === 0 && (
                  <div className="text-xs text-[var(--color-muted)]">No sessions in this lab.</div>
                )}
                {labSessions.map((s) => (
                  <div
                    key={s.id}
                    className="flex items-center justify-between text-xs px-2 py-1.5 rounded-lg bg-[var(--color-tile)] border border-[var(--color-border)]"
                  >
                    <span className="text-[var(--color-label-secondary)]">{s.title || s.id}</span>
                    <span className="text-[var(--color-muted)] font-mono">
                      {`${typeof s.current_spend_usd === 'number' && Number.isFinite(s.current_spend_usd) ? fmtMoney(s.current_spend_usd) : '—'} / ${typeof s.spend_limit_usd === 'number' && Number.isFinite(s.spend_limit_usd) ? fmtMoney(s.spend_limit_usd) : '—'} · ${s.status}`}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
