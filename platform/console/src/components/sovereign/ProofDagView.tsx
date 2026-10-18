'use client';

import React, { useMemo, useState } from 'react';
import { GitBranch } from 'lucide-react';
import { CampaignState } from '@/types';
import { fetchQirCompile, type QirCompileReceipt } from '@/lib/api';
import { asRecord } from '@/lib/guards';

export { asRecord };

interface ProofDagViewProps {
  campaign?: CampaignState | null;
}

interface DagNode {
  id: string;
  label: string;
  kind: string;
  angle?: number;
}

interface DagEdge {
  id: string;
  source: string;
  target: string;
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

// Caps rendered IR at 300 lines with the cut labeled — the receipt stays
// complete server-side; only the display is bounded.
function receiptLineCapped(llvmIr?: string): string {
  if (!llvmIr) return 'no IR returned';
  const lines = llvmIr.split('\n');
  if (lines.length <= 300) return llvmIr;
  return `${lines.slice(0, 300).join('\n')}\n…[display truncated: ${lines.length} lines total]`;
}

/**
 * Layered proof DAG: nodes laid out by longest-path layer over the REAL
 * edge set (sources left, sinks right), edges drawn as connectors.
 * Clicking a node shows its record + the campaign provenance receipt —
 * every element traces to something, per the provenance rule.
 */
export function ProofDagView({ campaign }: ProofDagViewProps) {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [compileOpen, setCompileOpen] = useState(false);
  const [receipt, setReceipt] = useState<QirCompileReceipt | null | undefined>(undefined);
  const [compileError, setCompileError] = useState<string | null>(null);
  const [compiling, setCompiling] = useState(false);

  const campaignKey = `${campaign?.session_id ?? ''}::${campaign?.quantum_factors?.qasm ?? ''}`;
  const [prevCampaignKey, setPrevCampaignKey] = useState(campaignKey);
  if (prevCampaignKey !== campaignKey) {
    setPrevCampaignKey(campaignKey);
    setSelectedId(null);
    setReceipt(undefined);
    setCompileError(null);
  }

  const wireCuts = useMemo(() => {
    const raw = (campaign?.quantum_factors?.wire_cuts ?? []) as unknown[];
    return raw.map((w) => asRecord(w));
  }, [campaign?.quantum_factors?.wire_cuts]);

  // Campaign signals: stuck / rescoped / contested states rendered as
  // chips from resident data (trajectory watchdogs, branch statuses,
  // epistemic verdict). Per-node mapping would invent correspondence
  // between optimizer steps and circuit gates, so signals stay at the
  // campaign level with honest labels.
  const signals = useMemo(() => {
    const out: Array<{ tone: 'red' | 'amber' | 'rose'; text: string; title: string }> = [];
    for (const t of campaign?.trajectory ?? []) {
      const w = String(t.watchdog || '');
      const up = w.toUpperCase();
      if (up.includes('TRIP') || up.includes('STUCK') || up.includes('BARREN')) {
        out.push({ tone: 'red', text: `stuck: step ${t.step} (${w})`, title: 'Watchdog trip recorded in trajectory' });
      } else if (up.includes('RESCOP') || up.includes('ROLLBACK')) {
        out.push({ tone: 'amber', text: `rescoped: step ${t.step} (${w})`, title: 'Optimizer rescoped after this step' });
      }
    }
    for (const b of campaign?.branches ?? []) {
      const r = asRecord(b);
      const status = String(r.status || '');
      if (/rescop|stuck|failed/i.test(status)) {
        out.push({ tone: 'amber', text: `branch ${String(r.id || r.name || '?')}: ${status}`, title: 'Branch did not converge cleanly' });
      }
    }
    const ep = String(campaign?.epistemics?.status || '');
    if (/CONTESTED|FAILED/i.test(ep)) {
      out.push({ tone: 'rose', text: `epistemics: ${ep}`, title: 'Claim verdict is contested, not verified' });
    }
    return out;
  }, [campaign?.trajectory, campaign?.branches, campaign?.epistemics?.status]);

  const runCompile = async () => {
    const qasm = campaign?.quantum_factors?.qasm;
    if (!qasm || compiling) return;
    setCompiling(true);
    setCompileError(null);
    try {
      // Null = backend refusal (HTTP !ok). Throw = transport failure —
      // rendered distinctly so a down bridge never reads as "refused".
      setReceipt(await fetchQirCompile(qasm));
    } catch (e) {
      setReceipt(undefined);
      setCompileError(e instanceof Error ? e.message : String(e));
    } finally {
      setCompiling(false);
    }
  };

  const graph = useMemo(() => {
    const rawNodes = (campaign?.quantum_factors?.dag_nodes ?? []) as unknown[];
    const rawEdges = (campaign?.quantum_factors?.dag_edges ?? []) as unknown[];
    const nodes: DagNode[] = rawNodes.map((n, i) => {
      const r = asRecord(n);
      const data = asRecord(r.data);
      return {
        id: str(r.id) || `node-${i}`,
        label: str(data.label) || str(r.label) || str(r.name) || `node-${i}`,
        kind: str(data.type) || str(r.type) || 'node',
        angle: typeof data.angle === 'number' ? data.angle : undefined,
      };
    });
    const ids = new Set(nodes.map((n) => n.id));
    const edges: DagEdge[] = rawEdges
      .map((e, i) => {
        const r = asRecord(e);
        return { id: str(r.id) || `e-${i}`, source: str(r.source), target: str(r.target) };
      })
      .filter((e) => e.source && e.target && ids.has(e.source) && ids.has(e.target));

    // Longest-path layering from roots (nodes with no in-edges).
    const inDeg = new Map<string, number>(nodes.map((n) => [n.id, 0]));
    const out = new Map<string, string[]>();
    for (const e of edges) {
      inDeg.set(e.target, (inDeg.get(e.target) ?? 0) + 1);
      const list = out.get(e.source) ?? [];
      list.push(e.target);
      out.set(e.source, list);
    }
    const layer = new Map<string, number>();
    const roots = nodes.filter((n) => (inDeg.get(n.id) ?? 0) === 0).map((n) => n.id);
    const queue: string[] = [...roots];
    // Cycle guard: no roots with edges present means every node sits in a
    // cycle — seed from the first node but flag it loudly instead of
    // rendering a confident layout.
    const hasCycleSeed = queue.length === 0 && nodes.length > 0 && edges.length > 0;
    if (queue.length === 0 && nodes.length > 0) queue.push(nodes[0].id);
    for (const id of queue) layer.set(id, 0);
    const order = [...queue];
    for (let h = 0; h < order.length; h++) {
      const id = order[h];
      for (const next of out.get(id) ?? []) {
        const cand = (layer.get(id) ?? 0) + 1;
        if (cand > (layer.get(next) ?? -1)) layer.set(next, cand);
        if (!order.includes(next)) order.push(next);
      }
    }
    for (const n of nodes) {
      if (!layer.has(n.id)) layer.set(n.id, 0);
    }
    // Any node unreachable from the seed set, or a seed forced from a
    // rootless graph, indicates a cycle / disconnected loop.
    const hasCycle = hasCycleSeed || order.length < nodes.length;

    const COL_X = 190;
    const ROW_Y = 62;
    const columns = new Map<number, string[]>();
    for (const n of nodes) {
      const l = layer.get(n.id) ?? 0;
      const col = columns.get(l) ?? [];
      col.push(n.id);
      columns.set(l, col);
    }
    const pos = new Map<string, { x: number; y: number }>();
    for (const [l, col] of columns) {
      col.forEach((id, i) => pos.set(id, { x: 30 + l * COL_X, y: 30 + i * ROW_Y }));
    }
    const maxLayer = columns.size > 0 ? Math.max(...columns.keys()) : 0;
    const maxRows = columns.size > 0 ? Math.max(...[...columns.values()].map((c) => c.length)) : 1;
    return { nodes, edges, pos, width: 30 + maxLayer * COL_X + 220, height: Math.max(120, 30 + maxRows * ROW_Y), hasCycle };
  }, [campaign?.quantum_factors?.dag_nodes, campaign?.quantum_factors?.dag_edges]);

  const selected = graph.nodes.find((n) => n.id === selectedId) ?? null;
  const provenance = campaign?.notebook?.provenance_hash;

  if (graph.nodes.length === 0) {
    return (
      <div className="absolute inset-0 flex items-center justify-center">
        <div className="flex flex-col items-center gap-3 text-center px-6">
          <GitBranch className="w-8 h-8 text-slate-600" />
          <p className="text-xs text-slate-400 font-code">No proof DAG in this campaign yet.</p>
          <p className="text-[11px] text-slate-600">Run the compiler probe to synthesize a circuit DAG.</p>
        </div>
      </div>
    );
  }

  return (
    <div data-testid="proof-dag" className="absolute inset-0 flex flex-col">
      <div className="pt-20 pb-1 text-center font-code text-[10px] text-slate-500 shrink-0">
        {graph.nodes.length} nodes · {graph.edges.length} edges · {campaign?.target_system || 'untargeted'}
      </div>
      {/* Wire-cut ledger: planned template vs measured cut jobs. Unmeasured
          cuts render their known fields with a planned prefix — never a bare
          generic chip that hides what was templated. */}
      {wireCuts.length > 0 && (
        <div className="px-6 pb-1 flex flex-wrap justify-center gap-1.5 shrink-0">
          {wireCuts.map((w, i) => {
            const measured = w.measured === true;
            const frags = Array.isArray(w.fragments) ? w.fragments.length : 0;
            const cuts = typeof w.total_cuts === 'number' && Number.isFinite(w.total_cuts) ? w.total_cuts : null;
            if (measured) {
              return (
                <span
                  key={i}
                  title={`Cut job ${String(w.job_id ?? '?')}`}
                  className="px-2 py-0.5 rounded-full font-code text-[10px] border bg-emerald-400/10 border-emerald-400/40 text-emerald-300"
                >
                  {`measured: ${cuts ?? '?'} cuts · ${frags} fragments · job ${String(w.job_id ?? '?').slice(0, 8)}`}
                </span>
              );
            }
            const known: string[] = [];
            if (w.cut_type != null) known.push(`type ${String(w.cut_type).slice(0, 24)}`);
            if (cuts != null) known.push(`${cuts} cuts`);
            if (typeof w.overhead === 'string' && w.overhead) known.push(`overhead ${w.overhead.slice(0, 32)}`);
            if (frags > 0) known.push(`${frags} fragments`);
            return (
              <span
                key={i}
                title="Synthesis placeholder — no cut has executed"
                className="px-2 py-0.5 rounded-full font-code text-[10px] border bg-amber-400/10 border-amber-400/40 text-amber-300"
              >
                {known.length > 0 ? `planned: ${known.join(' · ')} (not executed)` : 'planned cut (not executed)'}
              </span>
            );
          })}
        </div>
      )}
      {graph.hasCycle && (
        <div className="px-6 pb-1 flex justify-center shrink-0">
          <span
            title="The edge set contains a cycle — layering is a fallback, not a proof order"
            className="px-2 py-0.5 rounded-full font-code text-[10px] border bg-rose-400/10 border-rose-400/40 text-rose-300"
          >
            cycle detected — layout is fallback, not proof order
          </span>
        </div>
      )}
      {/* Campaign signals: stuck / rescoped / contested states. */}
      {signals.length > 0 && (
        <div className="px-6 pb-1 flex flex-wrap justify-center gap-1.5 shrink-0">
          {signals.map((s, i) => (
            <span
              key={i}
              title={s.title}
              className={`px-2 py-0.5 rounded-full font-code text-[10px] border ${
                s.tone === 'red'
                  ? 'bg-red-400/10 border-red-400/40 text-red-300'
                  : s.tone === 'amber'
                    ? 'bg-amber-400/10 border-amber-400/40 text-amber-300'
                    : 'bg-rose-400/10 border-rose-400/40 text-rose-300'
              }`}
            >
              {s.text}
            </span>
          ))}
        </div>
      )}
      <div className="flex-1 overflow-auto px-6 pb-4">
        <svg
          width={graph.width}
          height={graph.height}
          className="mx-auto max-w-none"
          role="img"
          aria-label={`Proof DAG: ${graph.nodes.length} nodes, ${graph.edges.length} edges`}
        >
          <defs>
            <linearGradient id="dagEdge" x1="0%" y1="0%" x2="100%" y2="0%">
              <stop offset="0%" stopColor="#00E5FF" stopOpacity="0.55" />
              <stop offset="100%" stopColor="#00E5FF" stopOpacity="0.15" />
            </linearGradient>
          </defs>
          {graph.edges.map((e) => {
            const a = graph.pos.get(e.source);
            const b = graph.pos.get(e.target);
            if (!a || !b) return null;
            const x1 = a.x + 190;
            const y1 = a.y + 18;
            const x2 = b.x;
            const y2 = b.y + 18;
            const mx = (x1 + x2) / 2;
            return (
              <path
                key={e.id}
                d={`M ${x1} ${y1} C ${mx} ${y1}, ${mx} ${y2}, ${x2} ${y2}`}
                fill="none"
                stroke="url(#dagEdge)"
                strokeWidth="1.5"
              />
            );
          })}
          {graph.nodes.map((n) => {
            const p = graph.pos.get(n.id);
            if (!p) return null;
            const isSel = selectedId === n.id;
            return (
              <g
                key={n.id}
                transform={`translate(${p.x}, ${p.y})`}
                onClick={() => setSelectedId(isSel ? null : n.id)}
                className="cursor-pointer"
              >
                <rect
                  width="190"
                  height="36"
                  rx="10"
                  className={isSel ? 'fill-cyan-400/15 stroke-cyan-300' : 'fill-[#0E1117] stroke-cyan-400/25'}
                  strokeWidth="1"
                />
                <text x="12" y="15" className="fill-slate-100" fontSize="10" fontFamily="monospace">
                  {n.label.length > 26 ? `${n.label.slice(0, 25)}…` : n.label}
                </text>
                <text x="12" y="28" className="fill-slate-500" fontSize="9" fontFamily="monospace">
                  {n.kind}
                  {typeof n.angle === 'number' && Number.isFinite(n.angle) ? ` · θ=${n.angle.toFixed(3)}` : ''}
                </text>
              </g>
            );
          })}
        </svg>
      </div>
      <div className="shrink-0 px-6 pb-6">
        {selected ? (
          <div className="max-w-2xl mx-auto p-3.5 rounded-2xl bg-[#0E1117]/95 border border-cyan-400/25 space-y-1.5">
            <div className="text-xs font-code text-cyan-200">{selected.label}</div>
            <div className="text-[10px] font-code text-slate-400">
              id {selected.id} · {selected.kind}
              {typeof selected.angle === 'number' && Number.isFinite(selected.angle) ? ` · θ=${selected.angle}` : ''}
            </div>
            <div className="text-[10px] font-code text-slate-500">
              {provenance ? `ledger receipt ${provenance}` : 'no provenance receipt on this campaign'}
            </div>
          </div>
        ) : (
          <p className="text-center text-[10px] font-code text-slate-600">
            Select a node to trace its record.
          </p>
        )}
        {/* Compile receipt: original QASM vs backend-transpiled QIR. */}
        <div className="max-w-2xl mx-auto mt-2 rounded-2xl bg-[#0E1117]/95 border border-white/10">
          <button
            onClick={() => setCompileOpen((v) => !v)}
            aria-expanded={compileOpen}
            className="w-full flex items-center justify-between px-3.5 py-2 text-[11px] font-code text-slate-300 hover:text-white transition-colors"
          >
            <span>Compile receipt (QASM → QIR via backend)</span>
            <span>{compileOpen ? '−' : '+'}</span>
          </button>
          {compileOpen && (
            <div className="px-3.5 pb-3.5 space-y-2">
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <div className="text-[10px] font-code text-slate-500 mb-1">ORIGINAL QASM (campaign)</div>
                  <pre className="max-h-40 overflow-auto text-[10px] font-code text-slate-300 bg-black/40 rounded-lg p-2 whitespace-pre-wrap">
                    {campaign?.quantum_factors?.qasm || 'no circuit in this campaign'}
                  </pre>
                </div>
                <div>
                  <div className="text-[10px] font-code text-slate-500 mb-1">TRANSPILED QIR (backend)</div>
                  {!receipt && !compileError && (
                    <button
                      onClick={() => void runCompile()}
                      disabled={compiling || !campaign?.quantum_factors?.qasm}
                      className="px-2.5 py-1.5 rounded-lg bg-cyan-500/15 hover:bg-cyan-500/25 border border-cyan-400/30 text-cyan-200 text-[11px] transition-all disabled:opacity-40"
                    >
                      {compiling ? 'Compiling…' : 'Compile via backend'}
                    </button>
                  )}
                  {receipt === null && !compileError && (
                    <p className="text-[11px] font-code text-rose-400">Compile refused — nothing transpiled.</p>
                  )}
                  {compileError && (
                    <p className="text-[11px] font-code text-amber-300">
                      Compile transport failed — bridge unreachable, not a refusal ({compileError.slice(0, 160)}). Nothing transpiled.
                    </p>
                  )}
                  {receipt && (
                    <div className="space-y-1.5">
                      <div className="text-[10px] font-code text-slate-400">
                        {receipt.compiler_used || 'unanalyzed'}
                        {receipt.fallback_estimate ? ' · ESTIMATE (not compiler-verified)' : ' · compiler-verified'}
                        {' · '}qubits {receipt.num_qubits}{receipt.fallback_estimate ? ' (EST)' : ''} · depth {receipt.depth}{receipt.fallback_estimate ? ' (EST)' : ''} · gates {receipt.gate_count}{receipt.fallback_estimate ? ' (EST)' : ''} · T {receipt.t_count}{receipt.fallback_estimate ? ' (EST)' : ''}
                      </div>
                      <pre className="max-h-40 overflow-auto text-[10px] font-code text-slate-300 bg-black/40 rounded-lg p-2 whitespace-pre-wrap">
                        {receiptLineCapped(receipt.llvm_ir)}
                      </pre>
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
