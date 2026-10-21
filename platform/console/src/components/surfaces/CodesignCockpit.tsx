'use client';

import React, { useEffect, useState } from 'react';
import { API_BASE, apiFetch, authHeaders, fetchCodesignVerified, scoreCodesignBudget } from '@/lib/api';
import type { CodesignBudgetResult, CodesignFrontierResult, CodesignVerifiedResult } from '@/lib/api';

// Circuit co-design cockpit (read-only scoring).
// Talks only to Go (:8080) read-only routes; validity ground truth stays
// Python-owned (kickmix evaluator). No job-submission endpoints (hard rule).
// Local-classical only; unreproduced paper gains are labeled as claimed.

function DqiBoundWidget() {
  const [eps, setEps] = useState('0.002434');
  const [f, setF] = useState('0.5763');
  const e = eps.trim() === '' ? NaN : Number(eps);
  const fv = f.trim() === '' ? NaN : Number(f);
  const loss = Number.isFinite(e) && Number.isFinite(fv) && e >= 0 && e <= 1 && fv >= 0 ? 4 * e * fv : null;
  return (
    <div className="mt-3 flex flex-wrap items-center gap-2">
      <input aria-label="Epsilon" value={eps} onChange={(ev) => setEps(ev.target.value)} className="w-28 rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs font-mono" />
      <input aria-label="F value" value={f} onChange={(ev) => setF(ev.target.value)} className="w-28 rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs font-mono" />
      {loss !== null && <span className="text-xs text-white font-mono">loss ≤ {loss.toExponential(3)}</span>}
    </div>
  );
}

export function CodesignCockpit() {
  const [toffoli, setToffoli] = useState('5');
  const [qubits, setQubits] = useState('10');
  const [kappa, setKappa] = useState('1');
  const [tq, setTq] = useState<number | null>(null);
  const [tqError, setTqError] = useState<string | null>(null);
  const [batch, setBatch] = useState('5,10\n4,12\n1348,115');
  const [sweep, setSweep] = useState<{ k1: number; k2: number; k3: number } | null>(null);
  const [sweepError, setSweepError] = useState<string | null>(null);
  const [budgetRows, setBudgetRows] = useState('carry,0.0000005,40\nhistory,0.0000024,2');
  const [budget, setBudget] = useState<CodesignBudgetResult | null>(null);
  const [budgetError, setBudgetError] = useState<string | null>(null);
  const [budgetBusy, setBudgetBusy] = useState(false);
  const [frontier, setFrontier] = useState<CodesignFrontierResult | null>(null);
  const [frontierError, setFrontierError] = useState<string | null>(null);
  const [verified, setVerified] = useState<CodesignVerifiedResult | null>(null);
  const [verifiedError, setVerifiedError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    fetchCodesignVerified().then((out) => {
      if (!live) return;
      if (out.ok) setVerified(out.result);
      else setVerifiedError(out.error);
    });
    return () => {
      live = false;
    };
  }, []);

  const parseKappa = (): number | null => {
    const k = Number(kappa);
    return Number.isInteger(k) && k >= 1 && k <= 3 ? k : null;
  };

  const scoreTQ = async () => {
    setTqError(null);
    if (parseKappa() === null) {
      setTq(null);
      setTqError('kappa must be 1, 2, or 3: refusing to score as κ=1');
      return;
    }
    try {
      const res = await apiFetch(
        `${API_BASE}/api/codesign/tq?toffoli=${encodeURIComponent(toffoli)}&qubits=${encodeURIComponent(qubits)}&kappa=${encodeURIComponent(kappa)}`,
        { cache: 'no-store', headers: authHeaders() }
      );
      if (!res.ok) {
        setTq(null);
        setTqError(`refused (HTTP ${res.status})`);
        return;
      }
      const data = await res.json();
      setTq(data.tq);
    } catch (err) {
      setTq(null);
      setTqError(String(err).slice(0, 200));
    }
  };

  const scoreFrontier = async () => {
    setFrontierError(null);
    if (parseKappa() === null) {
      setFrontier(null);
      setFrontierError('kappa must be 1, 2, or 3: refusing to score as κ=1');
      return;
    }
    try {
      const entries = batch
        .split('\n')
        .map((l) => l.trim())
        .filter(Boolean)
        .map((l) => {
          const [t, q] = l.split(',').map((s) => Number(s.trim()));
          return { toffoli: t, qubits: q };
        });
      const res = await apiFetch(`${API_BASE}/api/codesign/frontier`, {
        method: 'POST',
        headers: authHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ entries, kappa: Number(kappa) || 1 }),
      });
      if (!res.ok) {
        setFrontier(null);
        setFrontierError(`refused (HTTP ${res.status})`);
        return;
      }
      const data = await res.json();
      setFrontier({ kappa: data.kappa, best: data.best, entries: data.entries || [], cells: data.cells });
    } catch (err) {
      setFrontier(null);
      setFrontierError(String(err).slice(0, 200));
    }
  };

  const scoreSweep = async () => {
    setSweepError(null);
    try {
      const qs = (k: number) =>
        apiFetch(
          `${API_BASE}/api/codesign/tq?toffoli=${encodeURIComponent(toffoli)}&qubits=${encodeURIComponent(qubits)}&kappa=${k}`,
          { cache: 'no-store', headers: authHeaders() }
        );
      const [r1, r2, r3] = await Promise.all([qs(1), qs(2), qs(3)]);
      if (!r1.ok || !r2.ok || !r3.ok) {
        setSweep(null);
        setSweepError('refused by the scoring rail');
        return;
      }
      const [d1, d2, d3] = await Promise.all([r1.json(), r2.json(), r3.json()]);
      setSweep({ k1: d1.tq, k2: d2.tq, k3: d3.tq });
    } catch (err) {
      setSweep(null);
      setSweepError(String(err).slice(0, 200));
    }
  };

  const scoreBudgetServer = async () => {
    // Server-sealed Table-S4 bound (Python math behind the Go rail).
    // Shape is parsed client-side; the union is never computed locally.
    setBudgetError(null);
    setBudget(null);
    const entries: Array<{ source: string; per_use_rate: number; uses: number }> = [];
    try {
      for (const line of budgetRows.split('\n')) {
        const t = line.trim();
        if (!t) continue;
        const [src, rateS, usesS] = t.split(',').map((s) => s.trim());
        if (!src || rateS === undefined || usesS === undefined) throw new Error(`bad row ${t}`);
        const rate = Number(rateS);
        const uses = Number(usesS);
        if (!Number.isFinite(rate) || rate < 0 || rate > 1 || !Number.isInteger(uses) || uses < 0) {
          throw new Error(`bad row ${t}`);
        }
        entries.push({ source: src, per_use_rate: rate, uses });
      }
    } catch (err) {
      setBudgetError(err instanceof Error ? err.message.slice(0, 200) : String(err).slice(0, 200));
      return;
    }
    setBudgetBusy(true);
    try {
      const out = await scoreCodesignBudget(entries);
      if (!out.ok) {
        setBudgetError(out.error);
        return;
      }
      setBudget(out.result);
    } finally {
      setBudgetBusy(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="rounded-2xl border border-emerald-400/20 bg-emerald-500/5 p-4">
        <div className="text-xs font-semibold text-white">Verified RS decoder — read-only{verified ? ` · v${verified.version} · ${verified.generated}` : ''}</div>
        {!verified && !verifiedError && <div className="mt-2 text-[11px] text-slate-400">Loading verified results…</div>}
        {verifiedError && <div className="mt-2 text-xs text-rose-300">{verifiedError}</div>}
        {verified && (
          <div className="mt-2 text-xs text-slate-200">
            <div className="font-mono">
              {verified.total.toffoli.toLocaleString('en-US')}T × {verified.total.qubits.toLocaleString('en-US')}Q — composed end-to-end
            </div>
            <ul className="mt-2 space-y-0.5 font-mono text-[11px]">
              {verified.components.map((c) => (
                <li key={c.name} title={c.pin}>
                  {c.name}: {c.toffoli.toLocaleString('en-US')}T · {c.scope}
                </li>
              ))}
            </ul>
            <div className="mt-2 text-[11px] text-slate-400">{verified.labels.join(' · ')}</div>
            <details className="mt-2">
              <summary className="cursor-pointer text-[11px] text-cyan-200">Proofs ({verified.proofs.length})</summary>
              <ul className="mt-1 space-y-0.5 text-[11px] text-slate-300">
                {verified.proofs.map((p, i) => (
                  <li key={i}>{p}</li>
                ))}
              </ul>
            </details>
          </div>
        )}
      </div>

      <div className="rounded-2xl border border-white/10 bg-black/40 p-4">
        <div className="text-xs font-semibold text-white">TQ scoring — read-only</div>
        <div className="text-[11px] text-slate-400">TQ = T × Q^κ (κ∈[1,3]). Golden 5×10=50 pinned both sides. Unreproduced gains labeled claimed.</div>
        <div className="mt-3 flex flex-wrap gap-2">
          <input aria-label="Toffoli count" value={toffoli} onChange={(e) => setToffoli(e.target.value)} className="w-28 rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs" />
          <input aria-label="Qubit count" value={qubits} onChange={(e) => setQubits(e.target.value)} className="w-28 rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs" />
          <input aria-label="Kappa" value={kappa} onChange={(e) => setKappa(e.target.value)} className="w-20 rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs" />
          <button onClick={scoreTQ} className="rounded-lg bg-cyan-500/15 border border-cyan-400/30 px-3 py-1 text-xs text-cyan-200">Score</button>
        </div>
        {tq !== null && <div className="mt-2 text-xs text-white">TQ = {tq}</div>}
        {tqError && <div className="mt-2 text-xs text-rose-300">{tqError}</div>}
      </div>

      <div className="rounded-2xl border border-white/10 bg-black/40 p-4">
        <div className="text-xs font-semibold text-white">MAP-Elites frontier — read-only batch (≤64)</div>
        <div className="text-[11px] text-slate-400">One T,Q per line. Min-TQ per cell survives. Paper reference points: HHL 1348/115, Gaussian 1583/139.</div>
        <textarea aria-label="Frontier batch" value={batch} onChange={(e) => setBatch(e.target.value)} rows={4} className="mt-3 w-full rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs font-mono" />
        <button onClick={scoreFrontier} className="mt-2 rounded-lg bg-cyan-500/15 border border-cyan-400/30 px-3 py-1 text-xs text-cyan-200">Score frontier</button>
        {frontierError && <div className="mt-2 text-xs text-rose-300">{frontierError}</div>}
        {frontier && (
          <div className="mt-2 text-xs text-slate-200">
            <div>cells: {frontier.cells} · best: {frontier.best ? `${frontier.best.toffoli}T × ${frontier.best.qubits}Q = ${frontier.best.tq}` : 'none'}</div>
            <ul className="mt-1 space-y-0.5 font-mono">
              {frontier.entries.slice(0, 12).map((e, i) => (
                <li key={i}>{e.toffoli}T × {e.qubits}Q = {e.tq}</li>
              ))}
            </ul>
          </div>
        )}
      </div>

      <div className="rounded-2xl border border-white/10 bg-black/40 p-4">
        <div className="text-xs font-semibold text-white">Kappa sweep + failure-budget — read-only</div>
        <div className="text-[11px] text-slate-400">κ=1..3 via the Go rail (same formula both sides). Sweep always covers all three κ — the κ field above is not used here. Budget is the server-sealed Table-S4 union bound (Python math, Go rail).</div>
        <div className="mt-3 flex flex-wrap gap-2">
          <button onClick={scoreSweep} className="rounded-lg bg-cyan-500/15 border border-cyan-400/30 px-3 py-1 text-xs text-cyan-200">Sweep κ=1..3</button>
          {sweep && <span className="text-xs text-white font-mono">κ1={sweep.k1} κ2={sweep.k2} κ3={sweep.k3}</span>}
          {sweepError && <span className="text-xs text-rose-300">{sweepError}</span>}
        </div>
        <textarea aria-label="Budget rows" value={budgetRows} onChange={(e) => setBudgetRows(e.target.value)} rows={2} className="mt-3 w-full rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs font-mono" />
        <button onClick={scoreBudgetServer} disabled={budgetBusy} className="mt-2 rounded-lg bg-cyan-500/15 border border-cyan-400/30 px-3 py-1 text-xs text-cyan-200 disabled:opacity-50">
          {budgetBusy ? 'Scoring…' : 'Seal union bound'}
        </button>
        {budget && (
          <div className="mt-2 text-xs text-white font-mono">
            <ul className="space-y-0.5">
              {budget.rows.map((r, i) => (
                <li key={i}>{r.source}: {r.per_use_rate} × {r.uses} = {r.contribution.toExponential(3)}</li>
              ))}
            </ul>
            <div className="mt-1">union ≤ {budget.union_bound.toExponential(3)} (server-sealed)</div>
          </div>
        )}
        {budgetError && <div className="mt-2 text-xs text-rose-300">{budgetError}</div>}
      </div>

      <details className="rounded-2xl border border-white/10 bg-black/40 p-4 text-[11px] text-slate-300">
        <summary className="cursor-pointer text-xs font-semibold text-white">Families (anti-caution fix) — process note</summary>
        <div className="mt-1">N distinct tracks mature separately, then share elites. Stagnation = offers-since-improvement per family; the widen-nudge names the most stagnant track. Full lineage + elite pool land with the Go migration (bridge RPC); this cockpit shows scoring only until then.</div>
      </details>

      <details className="rounded-2xl border border-white/10 bg-black/40 p-4">
        <summary className="cursor-pointer text-xs font-semibold text-white">DQI loss bound (Eq 25) — local estimate</summary>
        <div className="text-[11px] text-slate-400 mt-1">|⟨f⟩impl−⟨f⟩| ≤ 4ε⟨f⟩. Paper gate 100/100k → ε≤2.434e−3; RS 0.5763/Prange 0.5009 → ≤7.4% of advantage.</div>
        <DqiBoundWidget />
      </details>

      <details className="rounded-2xl border border-white/10 bg-black/40 p-4 text-[11px] text-slate-300">
        <summary className="cursor-pointer text-xs font-semibold text-white">Domain packs — scaffold registry</summary>
        <ul className="mt-1 space-y-0.5 font-mono">
          {['numerics', 'inversion', 'dqi', 'chemistry', 'gauge'].map((p) => (
            <li key={p}>{p} · v0.1.0 · scaffold · anchor-required</li>
          ))}
        </ul>
        <div className="mt-1 text-slate-400">Promotion to active needs anchor_ref + proof_ref in the ledger. A pack name is not a result.</div>
      </details>

      <div className="rounded-2xl border border-amber-400/20 bg-amber-500/5 p-4 text-[11px] text-amber-200">
        Validity ground truth is the Python kickmix evaluator (simulator-only, Aer local). This cockpit scores numbers only — never verdicts. Paper gains (12×/32×/39×/…) are Anthropic-claimed until independently rerun.
      </div>
    </div>
  );
}
