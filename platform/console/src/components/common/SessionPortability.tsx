'use client';

import React, { useRef, useState } from 'react';
import {
  downloadFile,
  exportLedger,
  exportSession,
  importSession,
  type LedgerExportFormat,
} from '@/lib/api';

// U2-a: session + ledger portability (FlightRecorder home). Export
// downloads the manifest/ledger as a file; import reads a manifest file,
// posts it, and hands the resulting session to the parent (which switches
// to it). Import validates JSON client-side first (never posts unparseable
// text); every backend refusal renders verbatim; transport failure is
// named as unreachable, never merged with refusal.
export function SessionPortability({
  sessionId,
  onSessionImported,
}: {
  sessionId: string | null;
  onSessionImported: (sessionId: string, replayed: boolean) => void;
}) {
  const [busy, setBusy] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement | null>(null);

  const run = async (kind: string, fn: () => Promise<{ ok: true } | { error: string } | null>) => {
    setBusy(kind);
    setNotice(null);
    try {
      const res = await fn();
      if (res === null) {
        setNotice('backend unreachable — nothing changed.');
      } else if ('error' in res) {
        setNotice(res.error);
      }
    } finally {
      setBusy(null);
    }
  };

  const doExportSession = () =>
    run('export-session', async () => {
      if (!sessionId) return { error: 'no active session to export.' };
      const res = await exportSession(sessionId);
      if (res === null) return null;
      if ('error' in res) return res;
      const text = JSON.stringify(res.manifest, null, 2);
      if (!downloadFile(`session-${sessionId.slice(0, 8)}.json`, text, 'application/json')) {
        return { error: 'download needs a browser — manifest held in memory instead.' };
      }
      return { ok: true as const };
    });

  const doExportLedger = (format: LedgerExportFormat) =>
    run(`export-ledger-${format}`, async () => {
      if (!sessionId) return { error: 'no active session to export.' };
      const res = await exportLedger(sessionId, format);
      if (res === null) return null;
      if ('error' in res) return res;
      const text =
        format === 'json'
          ? JSON.stringify(res.events ?? [], null, 2)
          : (res.markdown ?? '(empty ledger)');
      const ext = format === 'json' ? 'json' : 'md';
      if (!downloadFile(`ledger-${sessionId.slice(0, 8)}.${ext}`, text, format === 'json' ? 'application/json' : 'text/markdown')) {
        return { error: 'download needs a browser — export held in memory instead.' };
      }
      return { ok: true as const };
    });

  const onFile = async (file: { text: () => Promise<string>; name?: string } | null | undefined) => {
    if (!file) return;
    setBusy('import');
    setNotice(null);
    try {
      let raw: string;
      try {
        raw = await file.text();
      } catch {
        setNotice('could not read that file — nothing sent.');
        return;
      }
      let manifest: unknown;
      try {
        manifest = JSON.parse(raw);
      } catch {
        setNotice('not valid JSON — nothing sent to the backend.');
        return;
      }
      const res = await importSession(manifest);
      if (res === null) {
        setNotice('backend unreachable — nothing changed.');
      } else if ('error' in res) {
        setNotice(res.error);
      } else {
        onSessionImported(res.session_id, res.replayed);
      }
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-2" aria-label="Session portability">
      <div className="text-xs font-semibold text-white">Session portability · export / import</div>
      <div className="flex flex-wrap items-center gap-1.5 text-[11px]">
        <button
          type="button"
          disabled={busy !== null || !sessionId}
          onClick={() => void doExportSession()}
          aria-label="Export session manifest"
          className="px-2 py-1 rounded-lg bg-white/5 hover:bg-white/10 border border-white/10 text-slate-200 disabled:opacity-40"
        >
          Export session
        </button>
        <button
          type="button"
          disabled={busy !== null || !sessionId}
          onClick={() => void doExportLedger('json')}
          aria-label="Export ledger as JSON"
          className="px-2 py-1 rounded-lg bg-white/5 hover:bg-white/10 border border-white/10 text-slate-200 disabled:opacity-40"
        >
          Ledger JSON
        </button>
        <button
          type="button"
          disabled={busy !== null || !sessionId}
          onClick={() => void doExportLedger('markdown')}
          aria-label="Export ledger as Markdown"
          className="px-2 py-1 rounded-lg bg-white/5 hover:bg-white/10 border border-white/10 text-slate-200 disabled:opacity-40"
        >
          Ledger Markdown
        </button>
        <button
          type="button"
          disabled={busy !== null}
          onClick={() => fileRef.current?.click()}
          aria-label="Import session manifest"
          className="px-2 py-1 rounded-lg bg-white/5 hover:bg-white/10 border border-white/10 text-slate-200 disabled:opacity-40"
        >
          Import session
        </button>
        <input
          ref={fileRef}
          type="file"
          accept="application/json,.json"
          className="hidden"
          aria-label="Session manifest file"
          onChange={(e) => {
            const f = e.target.files?.[0];
            e.target.value = '';
            void onFile(f ?? null);
          }}
        />
      </div>
      {notice && (
        <p className="text-[11px] text-amber-200" role="alert">
          {notice}
        </p>
      )}
    </div>
  );
}
