'use client';

import React, { useEffect, useState } from 'react';
import { API_BASE, apiFetch, authHeaders, evaluateCodesignArtifact, fetchCodesignPacks, fetchCodesignVerified, runFamilySearch, scoreCodesignBudget } from '@/lib/api';
import type { CodesignBudgetResult, CodesignFamiliesResult, CodesignFrontierResult, CodesignPacksResult, CodesignVerifiedResult } from '@/lib/api';

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
  const [packs, setPacks] = useState<CodesignPacksResult | null>(null);
  const [packsError, setPacksError] = useState<string | null>(null);
  const [evalSpec, setEvalSpec] = useState('and2');
  const [evalArtifact, setEvalArtifact] = useState('[["ccx",[0,1,2],[]]]');
  const [evalTests, setEvalTests] = useState('64');
  const [evalKappa, setEvalKappa] = useState('1');
  const [evalSeed, setEvalSeed] = useState('7');
  const [evalFactory, setEvalFactory] = useState('');
  const [evalDistance, setEvalDistance] = useState('');
  const [evalCycle, setEvalCycle] = useState('');
  const [evalRes, setEvalRes] = useState<Record<string, unknown> | null>(null);
  const [evalSpecKind, setEvalSpecKind] = useState<string | null>(null);
  const [evalError, setEvalError] = useState<string | null>(null);
  const [evalBusy, setEvalBusy] = useState(false);
  const [famSpec, setFamSpec] = useState('and2');
  const [famFamilies, setFamFamilies] = useState('direct,tower,mbu');
  const [famOffers, setFamOffers] = useState('[{"family":"direct","artifact":[["ccx",[0,1,2],[]]]},{"family":"tower","artifact":[["cx",[0,2],[]]]},{"family":"mbu","artifact":[["ccx",[0,1,2],[]]]}]');
  const [famRounds, setFamRounds] = useState('1');
  const [famKappa, setFamKappa] = useState('1');
  const [famRes, setFamRes] = useState<CodesignFamiliesResult | null>(null);
  const [famError, setFamError] = useState<string | null>(null);
  const [famBusy, setFamBusy] = useState(false);

  useEffect(() => {
    let live = true;
    fetchCodesignVerified().then((out) => {
      if (!live) return;
      if (out.ok) setVerified(out.result);
      else setVerifiedError(out.error);
    });
    fetchCodesignPacks().then((out) => {
      if (!live) return;
      if (out.ok) setPacks(out.result);
      else setPacksError(out.error);
    });
    return () => {
      live = false;
    };
  }, []);

  const parseKappa = (): number | null => {
    const k = Number(kappa);
    return Number.isInteger(k) && k >= 1 && k <= 3 ? k : null;
  };

  // Presets are live-verified valid artifacts (measured, not assumed).
  const evalPresets: Record<string, string> = {
    'and2': '[["ccx",[0,1,2],[]]]',
    'cz-phase': '[["cz",[0,1],[]]]',
    'copy1': '[["cx",[0,1],[]]]',
    'xor2': '[["cx",[0,2],[]],["cx",[1,2],[]]]',
  };

  const pickEvalSpec = (kind: string) => {
    setEvalSpec(kind);
    setEvalArtifact(evalPresets[kind] ?? '[]');
    setEvalRes(null);
    setEvalSpecKind(null);
    setEvalError(null);
  };

  const runEval = async () => {
    setEvalError(null);
    setEvalRes(null);
    setEvalSpecKind(null);
    let artifact: unknown;
    try {
      artifact = JSON.parse(evalArtifact);
    } catch {
      setEvalError('artifact must be JSON: refusing to send');
      return;
    }
    if (!Array.isArray(artifact)) {
      setEvalError('artifact must be a JSON array of [name, qubits, cond]: refusing to send');
      return;
    }
    if (evalFactory && (!evalDistance.trim() || !evalCycle.trim())) {
      setEvalError('factory diagnostics need code_distance + cycle_us (no silent physics): refusing to send');
      return;
    }
    setEvalBusy(true);
    try {
      const out = await evaluateCodesignArtifact({
        spec_kind: evalSpec,
        artifact,
        num_tests: Number(evalTests),
        max_failures: 0,
        kappa: Number(evalKappa),
        seed: Number(evalSeed),
        ...(evalFactory
          ? { factory: evalFactory, code_distance: Number(evalDistance), cycle_us: Number(evalCycle) }
          : {}),
      });
      if (!out.ok) {
        setEvalError(out.error);
        return;
      }
      setEvalSpecKind(out.spec_kind);
      setEvalRes(out.result);
    } finally {
      setEvalBusy(false);
    }
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

  const runFamilies = async () => {
    setFamError(null);
    setFamRes(null);
    let offers: unknown;
    try {
      offers = JSON.parse(famOffers);
    } catch {
      setFamError('offers must be JSON: refusing to send');
      return;
    }
    if (!Array.isArray(offers)) {
      setFamError('offers must be a JSON array of {family, artifact}: refusing to send');
      return;
    }
    const families = famFamilies.split(',').map((s) => s.trim()).filter(Boolean);
    setFamBusy(true);
    try {
      const out = await runFamilySearch({
        spec_kind: famSpec,
        families,
        kappa: Number(famKappa),
        offers,
        share_rounds: Number(famRounds),
      });
      if (!out.ok) {
        setFamError(out.error);
        return;
      }
      setFamRes(out.result);
    } finally {
      setFamBusy(false);
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
        <div className="mt-3 flex flex-wrap gap-2 items-center">
          <input aria-label="Toffoli count" value={toffoli} onChange={(e) => setToffoli(e.target.value)} className="w-28 rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs" />
          <input aria-label="Qubit count" value={qubits} onChange={(e) => setQubits(e.target.value)} className="w-28 rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs" />
          <label className="flex items-center gap-2 text-[11px] text-slate-400">
            κ
            <input aria-label="Kappa" type="range" min={1} max={3} step={1} value={kappa} onChange={(e) => setKappa(e.target.value)} className="w-20 accent-cyan-400" />
            <span className="font-mono text-white">{parseKappa() ?? '—'}</span>
          </label>
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
        <div className="text-xs font-semibold text-white">Evaluator runs — read-only (validity stays Python-owned)</div>
        <div className="text-[11px] text-slate-400">Named spec or truth table, artifact as JSON. Factory diagnostics attach the literature model (G3) — needs code distance + cycle time, no silent defaults.</div>
        <div className="mt-3 flex flex-wrap gap-2">
          <select aria-label="Spec kind" value={evalSpec} onChange={(e) => pickEvalSpec(e.target.value)} className="rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs text-white">
            {['and2', 'cz-phase', 'copy1', 'xor2'].map((k) => (
              <option key={k} value={k} className="bg-black">{k}</option>
            ))}
          </select>
          <input aria-label="Number of tests" value={evalTests} onChange={(e) => setEvalTests(e.target.value)} className="w-20 rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs font-mono" />
          <label className="flex items-center gap-1 text-[11px] text-slate-400">
            κ
            <input aria-label="Kappa" type="range" min={1} max={3} step={1} value={evalKappa} onChange={(e) => setEvalKappa(e.target.value)} className="w-16 accent-cyan-400" />
          </label>
          <input aria-label="Seed" value={evalSeed} onChange={(e) => setEvalSeed(e.target.value)} className="w-20 rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs font-mono" />
        </div>
        <textarea aria-label="Artifact JSON" value={evalArtifact} onChange={(e) => setEvalArtifact(e.target.value)} rows={2} className="mt-2 w-full rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs font-mono" />
        <div className="mt-2 flex flex-wrap gap-2 items-center">
          <select aria-label="Factory (optional)" value={evalFactory} onChange={(e) => setEvalFactory(e.target.value)} className="rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs text-white">
            <option value="" className="bg-black">no factory</option>
            <option value="gf2019_ccz" className="bg-black">gf2019_ccz</option>
          </select>
          <input aria-label="Code distance" value={evalDistance} onChange={(e) => setEvalDistance(e.target.value)} placeholder="d" className="w-16 rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs font-mono" />
          <input aria-label="Cycle time us" value={evalCycle} onChange={(e) => setEvalCycle(e.target.value)} placeholder="µs" className="w-16 rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs font-mono" />
          <button onClick={runEval} disabled={evalBusy} className="rounded-lg bg-cyan-500/15 border border-cyan-400/30 px-3 py-1 text-xs text-cyan-200 disabled:opacity-50">
            {evalBusy ? 'Running…' : 'Run evaluation'}
          </button>
        </div>
        {evalError && <div className="mt-2 text-xs text-rose-300">{evalError}</div>}
        {evalRes && (
          <div className="mt-2 text-xs text-slate-200 font-mono">
            <div>
              {evalRes.valid ? (
                <span className="text-emerald-400">VALID</span>
              ) : (
                <span className="text-rose-300">INVALID (refused, never scored)</span>
              )}
              {' '}{String(evalRes.toffoli)}T × {String(evalRes.peak_qubits)}Q
              {typeof evalRes.eps_bound_97_5 === 'number' && <span> · ε ≤ {evalRes.eps_bound_97_5.toExponential(3)}</span>}
            </div>
              <div className="mt-1 text-[11px] text-slate-400">
                depth {String(evalRes.depth)} · Toffoli-depth {String(evalRes.toffoli_depth)} · active {String(evalRes.active_qubits)} · spec {evalSpecKind}
              </div>
              {typeof evalRes.failures === 'number' && evalRes.failures > 0 && (
                <div className="mt-1 text-[11px] text-rose-200">
                  {String(evalRes.failures)} failure{evalRes.failures === 1 ? '' : 's'}
                  {Array.isArray(evalRes.failure_details) && evalRes.failure_details.length > 0 && (() => {
                    const d = evalRes.failure_details[0] as Record<string, unknown>;
                    return <> — first: trial {String(d.t)}, input {JSON.stringify(d.input)}, {String(d.reason).slice(0, 120)}</>;
                  })()}
                  {evalRes.failure_details_capped === true && <span className="text-slate-400"> (showing first 32 of {String(evalRes.failures)} — counts authoritative)</span>}
                </div>
              )}
            {!!evalRes.factory && typeof evalRes.factory === 'object' && (
              <div className="mt-1 text-[11px] text-slate-300">
                factory {(evalRes.factory as Record<string, unknown>).factory as string}:{' '}
                {String((evalRes.factory as Record<string, unknown>).serial_seconds_est)}s serial ·{' '}
                {(evalRes.factory as Record<string, unknown>).within_failure_budget ? 'within accuracy budget' : 'EXCEEDS accuracy budget'}
              </div>
            )}
          </div>
        )}
      </div>

      <div className="rounded-2xl border border-white/10 bg-black/40 p-4">
        <div className="text-xs font-semibold text-white">Families (anti-caution fix) — read-only</div>
        <div className="text-[11px] text-slate-400">N tracks mature separately on one spec, elites cross-pollinate, the widen-nudge names the most stagnant track. Human nudge reads the output — never auto-prompt-injection.</div>
        <div className="mt-3 flex flex-wrap gap-2">
          <select aria-label="Family spec" value={famSpec} onChange={(e) => setFamSpec(e.target.value)} className="rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs text-white">
            {['and2', 'cz-phase', 'copy1', 'xor2'].map((k) => (
              <option key={k} value={k} className="bg-black">{k}</option>
            ))}
          </select>
          <input aria-label="Families (comma list)" value={famFamilies} onChange={(e) => setFamFamilies(e.target.value)} className="w-40 rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs font-mono" />
          <input aria-label="Share rounds" value={famRounds} onChange={(e) => setFamRounds(e.target.value)} className="w-16 rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs font-mono" />
          <label className="flex items-center gap-1 text-[11px] text-slate-400">
            κ
            <input aria-label="Kappa" type="range" min={1} max={3} step={1} value={famKappa} onChange={(e) => setFamKappa(e.target.value)} className="w-16 accent-cyan-400" />
          </label>
          <button onClick={runFamilies} disabled={famBusy} className="rounded-lg bg-cyan-500/15 border border-cyan-400/30 px-3 py-1 text-xs text-cyan-200 disabled:opacity-50">
            {famBusy ? 'Running…' : 'Run families'}
          </button>
        </div>
        <textarea aria-label="Family offers JSON" value={famOffers} onChange={(e) => setFamOffers(e.target.value)} rows={3} className="mt-2 w-full rounded-lg bg-white/5 border border-white/10 px-2 py-1 text-xs font-mono" />
        {famError && <div className="mt-2 text-xs text-rose-300">{famError}</div>}
          {famRes && (
            <div className="mt-2 text-xs text-slate-200">
              <div className="font-mono">
                offers: {famRes.offers.map((o) => `${o.family} ${o.valid ? `${o.toffoli}T ${o.improved ? '✓' : '='}` : 'invalid'}`).join(' · ')}
              </div>
              <div className="mt-1 font-mono">elites: {Object.entries(famRes.elites).map(([f, e]) => `${f} ${e.toffoli}T×${e.qubits}Q`).join(' · ') || 'none yet'}</div>
            <div className="mt-1 font-mono text-[11px] text-slate-400">
              stagnation: {Object.entries(famRes.stagnation).map(([f, s]) => `${f}:${s}`).join(' ')} · lineage {famRes.lineage_len}
            </div>
            <div className="mt-1 text-[11px] text-amber-200">widen-nudge: {famRes.widen_nudge.nudge} ({famRes.widen_nudge.offers_since_improvement} offers since improvement)</div>
            <div className="mt-1 text-[11px] text-slate-300">
              sharing: {famRes.shares.map((s, i) => `round ${i + 1}: ${s.offers} offers → ${s.improvements} improvements`).join('; ') || 'no share rounds'}
            </div>
            <details className="mt-1">
              <summary className="cursor-pointer text-[11px] text-cyan-200">Lineage ({famRes.lineage_len} events{famRes.lineage.length < famRes.lineage_len ? `, showing last ${famRes.lineage.length}` : ''})</summary>
              <ul className="mt-1 space-y-0.5 font-mono text-[11px] text-slate-400">
                {famRes.lineage.map((e, i) => (
                  <li key={i}>
                    {String(e.family)} · {String(e.toffoli)}T {e.improved ? 'improved' : 'no improvement'}
                    {typeof e.parent === 'string' && e.parent !== '' ? ` · ← ${e.parent}` : ''}
                  </li>
                ))}
              </ul>
            </details>
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

      <details className="rounded-2xl border border-white/10 bg-black/40 p-4">
        <summary className="cursor-pointer text-xs font-semibold text-white">DQI loss bound (Eq 25) — local estimate</summary>
        <div className="text-[11px] text-slate-400 mt-1">|⟨f⟩impl−⟨f⟩| ≤ 4ε⟨f⟩. Paper gate 100/100k → ε≤2.434e−3; RS 0.5763/Prange 0.5009 → ≤7.4% of advantage.</div>
        <DqiBoundWidget />
      </details>

      <details className="rounded-2xl border border-white/10 bg-black/40 p-4 text-[11px] text-slate-300">
        <summary className="cursor-pointer text-xs font-semibold text-white">Domain packs — live registry (served, never hand-typed)</summary>
        {!packs && !packsError && <div className="mt-1 text-slate-400">Loading pack registry…</div>}
        {packsError && <div className="mt-1 text-rose-300">{packsError}</div>}
        {packs && (
          <ul className="mt-1 space-y-1 font-mono">
            {packs.order.map((name) => {
              const p = packs.packs.find((x) => x.name === name);
              if (!p) return null;
              return (
                <li key={name} title={p.evidence_reqs.join('; ')}>
                  {p.name} · v{p.version} · {p.status} · anchor-required{p.anchor_ref ? ` · ${p.anchor_ref}` : ''}{p.proof_ref ? ` · ${p.proof_ref}` : ''}
                </li>
              );
            })}
          </ul>
        )}
        <div className="mt-1 text-slate-400">Promotion to active needs anchor_ref + proof_ref in the ledger. A pack name is not a result.</div>
      </details>

      <div className="rounded-2xl border border-amber-400/20 bg-amber-500/5 p-4 text-[11px] text-amber-200">
        Validity ground truth is the Python kickmix evaluator (simulator-only, Aer local). This cockpit scores numbers only — never verdicts. Paper gains (12×/32×/39×/…) are Anthropic-claimed until independently rerun.
      </div>
    </div>
  );
}
