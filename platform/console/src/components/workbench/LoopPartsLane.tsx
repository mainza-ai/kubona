'use client';

import React, { useEffect, useState } from 'react';
import { cancelLoopSteer, fetchLoopSteers, steerLoop, updateLoopSteer } from '@/lib/api';
import { useLoopParts } from '@/domain/useLoopParts';
import { activePart, columnsOf } from '@/lib/run-columns';
import { needSentence } from '@/lib/need';
import type { LoopPart, LoopSteer } from '@/types';

/**
 * C1 — the run lane inside an agent-run card: time-ordered helper columns,
 * parallel helpers side by side, the active part marked, and a typed need
 * carried from the linked job. Parts are server-recorded; this renders them.
 */
export function LoopPartsLane({ loopId, live }: { loopId: string; live: boolean }) {
  // Live runs stream parts automatically; a settled run loads them only when
  // the reviewer asks, so a canvas of historical runs cannot storm the API.
  const [requested, setRequested] = useState(false);
  const enabled = live || requested;
  const { parts, status, error } = useLoopParts(loopId, enabled, live);

  if (!enabled) {
    return (
      <button
        type="button"
        className="k-run-parts-hint k-run-parts-open"
        data-testid="run-parts-open"
        onClick={(event) => {
          event.stopPropagation();
          setRequested(true);
        }}
        onPointerDown={(event) => event.stopPropagation()}
      >
        Show run parts
      </button>
    );
  }

  if (status === 'idle' || status === 'loading') {
    return (
      <div className="k-run-parts-hint" role="status" data-testid="run-parts-loading">
        Reading run parts…
      </div>
    );
  }
  if (status === 'refused') {
    return (
      <div className="k-run-parts-hint" role="status" title={error ?? undefined} data-testid="run-parts-refused">
        Run parts unavailable — the record is unchanged.
      </div>
    );
  }
  if (parts.length === 0) {
    return (
      <div className="k-run-parts-hint" data-testid="run-parts-empty">
        No parts recorded yet.
      </div>
    );
  }

  const columns = columnsOf(parts);
  const active = activePart(parts);

  return (
    <div className="k-run-parts" data-testid="run-parts" aria-label="Run parts">
      {columns.map((column) => (
        <section key={column.helper} className="k-run-column" aria-label={`${column.helper} parts`}>
          <header className="k-run-column-head">{column.helper.replace('_', ' ')}</header>
          <ul>
            {column.parts.map((part: LoopPart) => (
              <li
                key={part.part_id}
                className="k-run-part"
                data-state={part.state}
                data-active={active?.part_id === part.part_id ? 'true' : undefined}
              >
                <span className="k-run-part-dot" aria-hidden="true" />
                <span className="k-run-part-copy">
                  <span className="k-run-part-title">{part.title}</span>
                  {part.now ? <span className="k-run-part-now">{part.now}</span> : null}
                  {part.goal ? <span className="k-run-part-goal">{part.goal}</span> : null}
                  {part.need ? (
                    <span className="k-run-part-need" data-need={part.need.kind}>
                      {needSentence(part.need)}
                    </span>
                  ) : null}
                </span>
              </li>
            ))}
          </ul>
        </section>
      ))}
      {live && <LoopSteerBox loopId={loopId} active={active} partsCount={parts.length} />}
    </div>
  );
}

/**
 * C10 — steer a live run. The disclosure states exactly what the instruction
 * attaches to and when it takes effect; the request is an instruction record,
 * not an immediate tool call.
 */
function LoopSteerBox({ loopId, active, partsCount }: { loopId: string; active: LoopPart | null; partsCount: number }) {
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const [receipt, setReceipt] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [steers, setSteers] = useState<LoopSteer[]>([]);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editText, setEditText] = useState('');

  const reload = async () => {
    setSteers(await fetchLoopSteers(loopId));
  };

  useEffect(() => {
    let live = true;
    fetchLoopSteers(loopId).then((items) => {
      if (live) setSteers(items);
    });
    return () => {
      live = false;
    };
  }, [loopId, partsCount]);

  const submit = async () => {
    const trimmed = text.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    setError(null);
    const out = await steerLoop(loopId, trimmed);
    if (out.ok) {
      setReceipt(`Queued (${out.steer.id.slice(0, 8)}) — applied at the next step boundary.`);
      setText('');
      await reload();
    } else {
      setReceipt(null);
      setError(out.error);
    }
    setBusy(false);
  };

  const saveEdit = async (steerId: string) => {
    const trimmed = editText.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    setError(null);
    const out = await updateLoopSteer(loopId, steerId, trimmed);
    if (out.ok) {
      setReceipt(`Updated (${steerId.slice(0, 8)}) — the next boundary sees the edited instruction.`);
      setEditingId(null);
      setEditText('');
      await reload();
    } else {
      setError(out.error);
    }
    setBusy(false);
  };

  const cancel = async (steerId: string) => {
    if (busy) return;
    setBusy(true);
    setError(null);
    const out = await cancelLoopSteer(loopId, steerId);
    if (out.ok) {
      setReceipt(`Cancelled (${steerId.slice(0, 8)}) — it will not apply.`);
      await reload();
    } else {
      setError(out.error);
    }
    setBusy(false);
  };

  return (
    <div className="k-run-steer" data-testid="run-steer">
      <div className="k-run-steer-context">
        <span>
          Target: loop {loopId.slice(0, 8)} · {partsCount} part{partsCount === 1 ? '' : 's'}
          {active ? ` · active: ${active.title}` : ''}
        </span>
        <span>Applied at the next step boundary — not an immediate tool call.</span>
      </div>
      <div className="k-run-steer-row">
        <input
          aria-label="Steer this run"
          value={text}
          maxLength={500}
          placeholder="Instruction for the next step…"
          onChange={(event) => setText(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault();
              void submit();
            }
          }}
        />
        <button type="button" data-testid="run-steer-submit" disabled={busy || !text.trim()} onClick={() => void submit()}>
          {busy ? 'Queueing…' : 'Steer run'}
        </button>
      </div>
      {steers.length > 0 && (
        <ul className="k-run-steer-pending" aria-label="Queued steering instructions">
          {steers.map((steer) => (
            <li key={steer.id} data-testid={`queued-steer-${steer.id}`}>
              {editingId === steer.id ? (
                <span className="k-run-steer-edit">
                  <input
                    aria-label="Edit queued steering instruction"
                    value={editText}
                    maxLength={500}
                    onChange={(event) => setEditText(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter') {
                        event.preventDefault();
                        void saveEdit(steer.id);
                      }
                      if (event.key === 'Escape') {
                        setEditingId(null);
                        setEditText('');
                      }
                    }}
                  />
                  <button type="button" disabled={busy || !editText.trim()} onClick={() => void saveEdit(steer.id)}>
                    Save
                  </button>
                  <button type="button" disabled={busy} onClick={() => { setEditingId(null); setEditText(''); }}>
                    Cancel edit
                  </button>
                </span>
              ) : (
                <span className="k-run-steer-pending-row">
                  <span className="k-run-steer-pending-text">{steer.text}</span>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => {
                      setEditingId(steer.id);
                      setEditText(steer.text);
                    }}
                  >
                    Edit
                  </button>
                  <button type="button" disabled={busy} onClick={() => void cancel(steer.id)}>
                    Cancel
                  </button>
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
      {receipt && <p role="status" data-testid="run-steer-receipt">{receipt}</p>}
      {error && <p role="status" data-state="error" data-testid="run-steer-error">{error}</p>}
    </div>
  );
}
