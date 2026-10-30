'use client';

import React, { useEffect, useState } from 'react';
import { fetchCapabilities, fetchFidelity, type CapabilitiesPayload } from '@/lib/api';

export function SystemReadinessPanel() {
  const [caps, setCaps] = useState<CapabilitiesPayload | null>(null);
  const [device, setDevice] = useState('ibm_heron');
  const [depth, setDepth] = useState('12');
  const [twoQ, setTwoQ] = useState('8');
  const [fidelity, setFidelity] = useState<Record<string, unknown> | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    fetchCapabilities().then((c) => { if (live) setCaps(c); });
    return () => { live = false; };
  }, []);

  const assess = async () => {
    setError(null);
    const d = Number(depth);
    const q = Number(twoQ);
    if (!Number.isInteger(d) || d < 1 || !Number.isInteger(q) || q < 0) {
      setError('depth must be >=1 and two-qubit gates >=0.');
      return;
    }
    const out = await fetchFidelity(device, d, q);
    if (!out) { setError('fidelity assessment refused — nothing assumed.'); return; }
    setFidelity(out);
  };

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-black/40 p-4 space-y-2" aria-label="System readiness">
      <div>
        <div className="text-xs font-semibold text-white">Capabilities &amp; fidelity</div>
        <p className="text-[10px] text-slate-500">Runtime discovery plus the dispatch-grade floor assessment (no probe is run here).</p>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-[11px] font-mono text-slate-300">
        <div className="p-2 rounded-xl bg-white/[0.03] border border-white/[0.06] space-y-0.5">
          <div>catalog {caps?.catalog_digest?.slice(0, 16) ?? '—'}…</div>
          <div>harnesses {caps?.harnesses?.length ?? '—'} · tools {Array.isArray(caps?.tools) ? caps?.tools.length : '—'}</div>
          <div>reviewer {String(caps?.flags?.reviewer_configured ?? 'unknown')} · bridge {String(caps?.flags?.bridge_connected ?? 'unknown')}</div>
          <div>tool rounds {String(caps?.budget?.max_tool_rounds ?? '—')} · observation chars {String(caps?.budget?.max_observation_chars ?? '—')}</div>
        </div>
        <div className="p-2 rounded-xl bg-white/[0.03] border border-white/[0.06] space-y-1">
          <div className="flex gap-1">
            <input aria-label="Fidelity device" value={device} onChange={(e) => setDevice(e.target.value)} className="flex-1 min-w-0 bg-black/60 border border-white/10 rounded px-2 py-1" />
            <input aria-label="Fidelity depth" value={depth} onChange={(e) => setDepth(e.target.value)} className="w-12 bg-black/60 border border-white/10 rounded px-2 py-1" />
            <input aria-label="Fidelity two-qubit gates" value={twoQ} onChange={(e) => setTwoQ(e.target.value)} className="w-12 bg-black/60 border border-white/10 rounded px-2 py-1" />
            <button type="button" onClick={() => void assess()} className="px-2 py-1 rounded bg-cyan-500/15 border border-cyan-400/30 text-cyan-200">Assess</button>
          </div>
          {fidelity && (
            <div className="text-slate-400">
              admitted {String(fidelity.admitted ?? fidelity.skipped ?? 'unknown')} · forecast {String(fidelity.predicted_fidelity ?? '—')} · floor {String(fidelity.fidelity_floor ?? '—')} · probe_run {String(fidelity.probe_run ?? false)}
            </div>
          )}
        </div>
      </div>
      {error && <p role="alert" className="text-[11px] text-rose-300">{error}</p>}
    </div>
  );
}
