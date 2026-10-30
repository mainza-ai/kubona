'use client';

import React, { useState } from 'react';
import { createLab, registerTenantAdminKey } from '@/lib/api';
import { getOrCreateSigner } from '@/lib/custody';

// Multi-lab bootstrap: create the lab row, then register the browser key
// as its first admin in the same operator gesture. Both writes require the
// operator token; the token is held in component state only.
export function CreateLabPanel({ onCreated }: { onCreated: () => void }) {
  const [id, setId] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [created, setCreated] = useState<string | null>(null);

  const create = async () => {
    const labId = id.trim();
    if (!labId) { setNotice('lab id is required.'); return; }
    if (!token.trim()) { setNotice('operator token is required to register the first admin.'); return; }
    setBusy(true);
    setNotice(null);
    setCreated(null);
    try {
      const out = await createLab(labId, displayName.trim() || labId);
      if (!out.ok) { setNotice(`create refused: ${out.error}`); return; }
      const signer = await getOrCreateSigner();
      const admin = await registerTenantAdminKey(labId, signer.publicKeyHex, undefined, token);
      if (admin === null) { setNotice('lab created, but admin registration transport failed. Retry with the token.'); return; }
      if ('error' in admin) { setNotice(`lab created, but admin registration refused: ${admin.error}`); return; }
      setCreated(labId);
      setId('');
      setDisplayName('');
      setToken('');
      onCreated();
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-2" aria-label="Create laboratory">
      <div className="text-xs font-semibold text-white">Create laboratory</div>
      <p className="text-[10px] text-slate-500">Creates the lab and registers this browser key as its first admin. The operator token is used once and not stored.</p>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
        <input value={id} onChange={(e) => setId(e.target.value)} aria-label="New lab ID" placeholder="lab id (e.g. materials-lab)" className="bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-[11px] text-slate-200 font-mono" />
        <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} aria-label="New lab display name" placeholder="display name" className="bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-[11px] text-slate-200" />
      </div>
      <input value={token} onChange={(e) => setToken(e.target.value)} type="password" aria-label="New lab operator token" placeholder="operator token (KUBONA_API_TOKEN)" className="w-full bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-[11px] text-slate-200 font-mono" />
      <button type="button" onClick={() => void create()} disabled={busy || !id.trim() || !token.trim()} className="px-2 py-1 rounded-lg bg-cyan-500/15 border border-cyan-400/30 text-cyan-200 text-[11px] disabled:opacity-40">
        {busy ? 'Creating…' : 'Create lab + register admin'}
      </button>
      {created && <p role="status" className="text-[11px] text-emerald-200">Lab {created} created. Log in with this lab ID after switching lab.</p>}
      {notice && <p role="alert" className="text-[11px] text-amber-200">{notice}</p>}
    </div>
  );
}
