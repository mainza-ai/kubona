'use client';

import React, { useEffect, useState } from 'react';
import {
  authChallenge,
  authLogin,
  authLogout,
  authRevoke,
  authSessions,
  authWhoami,
  type AuthServerSession,
} from '@/lib/api';
import type { Lab } from '@/types';
import { signLoginChallengeWithCustody } from '@/lib/mandates';
import { peekSignerPubkeyHex } from '@/lib/custody';

// Session auth (A3/A4): challenge-response login bound to a lab admin key.
// The browser key (same in-memory keypair the mandate flows use) signs the
// server's nonce; the daemon verifies it against the lab's REGISTERED
// admin key and sets an httpOnly cookie. No passwords, no IdP — the lab
// admin key IS the identity (Q-13). Bootstrap a keyless lab below first
// (LabOpsPanel), then log in with it here.
export function AuthPanel({ labs }: { labs: Lab[] }) {
  const [selectedTenantId, setSelectedTenantId] = useState<string>('');
  const [whoami, setWhoami] = useState<{ authenticated: boolean; tenant_id?: string; pubkey_fp?: string } | null>(null);
  const [sessions, setSessions] = useState<AuthServerSession[] | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [browserKey, setBrowserKey] = useState<string | null>(null);

  const tenantId =
    selectedTenantId || (labs.find((l) => l.id === 'default') || labs[0])?.id || '';

  const refresh = async () => {
    const me = await authWhoami();
    setBrowserKey(await peekSignerPubkeyHex());
    setWhoami(me);
    if (me?.authenticated) {
      const list = await authSessions();
      setSessions(Array.isArray(list) ? list : null);
    } else {
      setSessions(null);
    }
  };

  useEffect(() => {
    let active = true;
    authWhoami()
      .then(async (me) => {
        if (!active) return;
        setBrowserKey(await peekSignerPubkeyHex());
        setWhoami(me);
        if (me?.authenticated) {
          const list = await authSessions();
          if (!active) return;
          setSessions(Array.isArray(list) ? list : null);
        } else {
          setSessions(null);
        }
      })
      .catch(() => {
        if (!active) return;
        setSessions(null);
      });
    return () => {
      active = false;
    };
  }, []);

  const run = async (kind: string, fn: () => Promise<unknown>) => {
    setBusy(kind);
    setNotice(null);
    try {
      await fn();
    } catch (e) {
      setNotice(`failed: ${e instanceof Error ? e.message : String(e)}`);
    } finally {
      setBusy(null);
    }
  };

  const login = () =>
    run('login', async () => {
      if (!tenantId) {
        setNotice('pick a lab first.');
        return;
      }
      // getOrCreateSigner mints (or reloads the durable non-extractable
      // key) only on this explicit operator action — never a render effect.
      const ch = await authChallenge();
      if (ch === null) {
        setNotice('backend unreachable — nothing sent.');
        return;
      }
      if ('error' in ch) {
        setNotice(`challenge refused: ${ch.error}`);
        return;
      }
      const { pubkey_hex, signature_hex } = await signLoginChallengeWithCustody(ch.nonce);
      setBrowserKey(pubkey_hex);
      const res = await authLogin(tenantId, pubkey_hex, ch.nonce, signature_hex);
      if (res === null) {
        setNotice('backend unreachable — nothing changed.');
      } else if ('error' in res) {
        setNotice(`login refused: ${res.error}`);
      }
      await refresh();
    });

  const logout = () =>
    run('logout', async () => {
      await authLogout();
      await refresh();
    });

  const revoke = (id: string) =>
    run(`revoke:${id}`, async () => {
      if (!confirm(`Revoke session ${id.slice(0, 8)}…? Its cookie stops working immediately.`)) return;
      const res = await authRevoke(id);
      if (res === null) {
        setNotice('backend unreachable — nothing changed.');
      } else if ('error' in res) {
        setNotice(`revoke refused: ${res.error}`);
      }
      await refresh();
    });

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/50 p-4 space-y-3" aria-label="Operator session">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-white">Operator session</span>
        {whoami?.authenticated ? (
          <span className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-emerald-500/20 text-emerald-200 border border-emerald-400/30">
            {whoami.tenant_id} · {whoami.pubkey_fp?.slice(0, 12)}…
          </span>
        ) : (
          <span className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-white/5 text-slate-400 border border-white/10">
            anonymous (bearer only)
          </span>
        )}
      </div>

      {!whoami?.authenticated && (
        <div className="flex flex-wrap items-center gap-1.5">
          <select
            value={tenantId}
            onChange={(e) => setSelectedTenantId(e.target.value)}
            aria-label="Login lab"
            className="bg-black/60 border border-white/10 rounded-lg px-2 py-1 text-[11px] text-slate-200"
          >
            {labs.map((l) => (
              <option key={l.id} value={l.id}>
                {l.display_name || l.id}
              </option>
            ))}
          </select>
          <button
            type="button"
            disabled={busy !== null || !tenantId}
            onClick={() => void login()}
            className="px-2 py-1 rounded-lg bg-cyan-500/15 border border-cyan-400/30 text-cyan-200 hover:bg-cyan-500/25 text-[11px] disabled:opacity-40"
          >
            {busy === 'login' ? 'Challenging…' : 'Log in with browser key'}
          </button>
          {!browserKey && (
            <span className="text-[11px] text-slate-500">
              No browser key yet — logging in mints a non-extractable Ed25519 key in this browser.
            </span>
          )}
        </div>
      )}

      {whoami?.authenticated && (
        <div className="space-y-2">
          <button
            type="button"
            disabled={busy !== null}
            onClick={() => void logout()}
            className="px-2 py-1 rounded-lg bg-white/5 hover:bg-white/10 border border-white/10 text-[11px] text-slate-300 disabled:opacity-40"
          >
            Log out (revokes this session)
          </button>
          <div>
            <div className="text-[11px] text-slate-400 mb-1">Live sessions in this lab (revoke to kill):</div>
            {sessions !== null && sessions.length === 0 && (
              <p className="text-[11px] text-slate-500">No live sessions listed — try Refresh.</p>
            )}
            <div className="space-y-1">
              {(sessions ?? []).map((s) => (
                <div
                  key={s.id}
                  className="flex items-center justify-between gap-2 px-2 py-1 rounded-lg bg-white/[0.03] text-[11px] font-mono"
                >
                  <span className="text-slate-300 truncate" title={s.id}>
                    {s.id.slice(0, 8)}… · key {s.pubkey_fp.slice(0, 12)}… · until {s.expires_at}
                  </span>
                  <button
                    type="button"
                    disabled={busy !== null}
                    onClick={() => void revoke(s.id)}
                    aria-label={`Revoke session ${s.id}`}
                    className="px-2 py-0.5 rounded-lg bg-rose-500/15 border border-rose-400/30 text-rose-200 hover:bg-rose-500/25 disabled:opacity-40 shrink-0"
                  >
                    Revoke
                  </button>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {notice && (
        <p className="text-[11px] text-amber-200" role="alert">
          {notice}
        </p>
      )}
    </div>
  );
}
