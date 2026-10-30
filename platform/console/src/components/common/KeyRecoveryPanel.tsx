'use client';

import React, { useState } from 'react';
import { authLogout, registerTenantAdminKey } from '@/lib/api';
import { createRecoverableSigner, getOrCreateSigner, installRecoveryEnvelope, peekSignerPubkeyHex } from '@/lib/custody';
import { signAdminActionWithSigner } from '@/lib/adminAction';
import { recoveryFilename, validateRecoveryEnvelope, type RecoveryEnvelope } from '@/lib/recovery';
import type { Lab } from '@/types';

// Offline recovery + rotation. The live key is non-extractable; rotation
// creates a new key with a passphrase-encrypted PKCS#8 kit, registers the
// new public key with the OLD key's signature, then commits the new key.
export function KeyRecoveryPanel({ labs }: { labs: Lab[] }) {
  const [tenant, setTenant] = useState(labs[0]?.id ?? '');
  const [passphrase, setPassphrase] = useState('');
  const [confirmPass, setConfirmPass] = useState('');
  const [restorePass, setRestorePass] = useState('');
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [currentKey, setCurrentKey] = useState<string | null>(null);

  React.useEffect(() => {
    peekSignerPubkeyHex().then(setCurrentKey);
  }, []);

  const rotate = async () => {
    setNotice(null);
    if (!tenant) { setNotice('select the lab to rotate.'); return; }
    if (passphrase.length < 12) { setNotice('passphrase must be at least 12 characters.'); return; }
    if (passphrase !== confirmPass) { setNotice('passphrases do not match.'); return; }
    setBusy(true);
    try {
      const old = await getOrCreateSigner();
      const next = await createRecoverableSigner(passphrase, tenant);

      // Download the kit BEFORE the server change: if the network outcome is
      // ambiguous, the new key is still recoverable offline.
      const blob = new Blob([JSON.stringify(next.recovery, null, 2)], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = recoveryFilename(tenant, next.signer.publicKeyHex);
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);

      await next.prepare();
      const action = await signAdminActionWithSigner({
        tenantId: tenant,
        action: 'set_admin_key',
        params: { new_admin_pubkey_hex: next.signer.publicKeyHex },
      }, old);
      const out = await registerTenantAdminKey(tenant, next.signer.publicKeyHex, action);
      if (out === null) {
        await next.rollback();
        setNotice('backend unreachable — old key restored; recovery kit downloaded for the attempted key.');
        return;
      }
      if ('error' in out) {
        await next.rollback();
        setNotice(`rotation refused: ${out.error} — old key restored; recovery kit downloaded.`);
        return;
      }
      next.activate();
      setCurrentKey(next.signer.publicKeyHex);
      setNotice('Rotated. Store the downloaded recovery kit offline; the old session was logged out.');
      setPassphrase('');
      setConfirmPass('');
      await authLogout();
    } catch (err) {
      setNotice(`rotation failed: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setBusy(false);
    }
  };

  const restore = async (file: File | null) => {
    if (!file) return;
    setNotice(null);
    setBusy(true);
    try {
      const parsed = JSON.parse(await file.text()) as RecoveryEnvelope;
      const env = validateRecoveryEnvelope(parsed);
      const signer = await installRecoveryEnvelope(env, restorePass);
      setCurrentKey(signer.publicKeyHex);
      setNotice(`Restored key ${signer.publicKeyHex.slice(0, 12)}… Log in again with this browser key.`);
      setRestorePass('');
      await authLogout();
    } catch (err) {
      setNotice(`restore failed: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-2" aria-label="Admin key recovery">
      <div>
        <div className="text-xs font-semibold text-white">Admin key recovery &amp; rotation</div>
        <p className="text-[10px] text-slate-500">The live key is non-extractable. Rotation creates a new key, signs the server change with the old key, and downloads an encrypted offline kit.</p>
        <p className="text-[10px] font-mono text-slate-400 mt-1">Current browser key: {currentKey ? `${currentKey.slice(0, 16)}…` : 'none minted yet'}</p>
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <select aria-label="Recovery lab" value={tenant} onChange={(e) => setTenant(e.target.value)} className="bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-[11px] text-slate-200 font-mono">
          {labs.length === 0 && <option value="">no labs</option>}
          {labs.map((l) => <option key={l.id} value={l.id}>{l.display_name || l.id}</option>)}
        </select>
        <input aria-label="Recovery passphrase" type="password" value={passphrase} onChange={(e) => setPassphrase(e.target.value)} placeholder="new passphrase (12+ chars)" className="flex-1 min-w-40 bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-[11px] text-slate-200" />
        <input aria-label="Confirm recovery passphrase" type="password" value={confirmPass} onChange={(e) => setConfirmPass(e.target.value)} placeholder="confirm passphrase" className="w-40 bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-[11px] text-slate-200" />
        <button type="button" onClick={() => void rotate()} disabled={busy || !tenant} className="px-2 py-1 rounded-lg bg-amber-500/15 border border-amber-400/30 text-amber-200 text-[11px] disabled:opacity-40">Rotate + download kit</button>
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <input aria-label="Recovery restore passphrase" type="password" value={restorePass} onChange={(e) => setRestorePass(e.target.value)} placeholder="passphrase for restore" className="flex-1 min-w-40 bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-[11px] text-slate-200" />
        <label className="px-2 py-1 rounded-lg bg-cyan-500/15 border border-cyan-400/30 text-cyan-200 text-[11px] cursor-pointer">
          Restore from kit
          <input type="file" accept="application/json,.json" aria-label="Recovery kit file" className="hidden" onChange={(e) => { const f = e.target.files?.[0] ?? null; e.target.value = ''; void restore(f); }} />
        </label>
      </div>
      {notice && <p role="status" className="text-[11px] text-slate-300 break-all">{notice}</p>}
    </div>
  );
}
