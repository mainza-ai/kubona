'use client';

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { GitBranch } from 'lucide-react';
import { CampaignState } from '@/types';
import { fetchQirCompile, type QirCompileReceipt } from '@/lib/api';
import { asRecord } from '@/lib/guards';
import { ProvenanceBadge } from '@/components/common/ProvenanceBadge';
import {
  buildFallbackDagLayout,
  DAG_NODE_H,
  DAG_NODE_W,
  type DagLayoutResult,
} from './proof-dag-layout';
import type { ProofDagWorkerRequest, ProofDagWorkerResponse } from './proof-dag.worker';

export { asRecord };

interface ProofDagViewProps {
  campaign?: CampaignState | null;
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

  const fallbackGraph = useMemo(() => {
    const rawNodes = (campaign?.quantum_factors?.dag_nodes ?? []) as unknown[];
    const rawEdges = (campaign?.quantum_factors?.dag_edges ?? []) as unknown[];
    return buildFallbackDagLayout(rawNodes, rawEdges, wireCuts);
  }, [campaign?.quantum_factors?.dag_nodes, campaign?.quantum_factors?.dag_edges, wireCuts]);

  const layoutKey = useMemo(
    () => `${fallbackGraph.nodes.map((n) => n.id).join(',')}|${fallbackGraph.edges.map((e) => `${e.source}->${e.target}`).join(',')}|${wireCuts.length}`,
    [fallbackGraph.nodes, fallbackGraph.edges, wireCuts.length],
  );
  const [workerGraph, setWorkerGraph] = useState<{ key: string; layout: DagLayoutResult } | null>(null);
  const layoutSeqRef = useRef(0);

  useEffect(() => {
    if (typeof Worker === 'undefined' || fallbackGraph.nodes.length === 0) {
      return;
    }
    const seq = (layoutSeqRef.current += 1);
    const activeKey = layoutKey;
    let worker: Worker | null = null;
    try {
      worker = new Worker(new URL('./proof-dag.worker.ts', import.meta.url), { type: 'module' });
      worker.onmessage = (event: MessageEvent<ProofDagWorkerResponse>) => {
        if (event.data.seq !== seq) return;
        if ('layout' in event.data) {
          setWorkerGraph({ key: activeKey, layout: event.data.layout });
        }
      };
      worker.onerror = () => {
        // Sync fallback remains active seamlessly on worker load/CSP issues.
      };
      const payload: ProofDagWorkerRequest = {
        seq,
        nodes: fallbackGraph.nodes,
        edges: fallbackGraph.edges,
        wireCuts,
        hasCycle: fallbackGraph.hasCycle,
      };
      worker.postMessage(payload);
    } catch {
      // Fallback already rendered synchronously on frame 1.
    }
    return () => {
      worker?.terminate();
    };
  }, [fallbackGraph, wireCuts, layoutKey]);

  const graph = workerGraph?.key === layoutKey ? workerGraph.layout : fallbackGraph;
  const selected = graph.nodes.find((n) => n.id === selectedId) ?? null;
  const provenance = campaign?.notebook?.provenance_hash;
  // The campaign currently stores the synthesized proposal DAG, not an
  // executed circuit graph. Keep the measured result in the Measurement /
  // Claim cards; never relabel a proposal as measured just because a notebook
  // exists.
  const proposedOnly = campaign?.notebook == null || campaign?.quantum_factors?.synthesized === true;
  const dagLabel = proposedOnly ? 'Proposed circuit DAG' : 'Executed circuit DAG';

  if (graph.nodes.length === 0) {
    return (
      <div className="absolute inset-0 flex items-center justify-center">
        <div className="flex flex-col items-center gap-3 text-center px-6">
          <GitBranch className="w-8 h-8 text-[var(--color-faint)]" />
          <p className="text-xs text-[var(--color-muted)] font-code">No circuit DAG is recorded for this campaign.</p>
          <p className="text-[12px] text-[var(--color-faint)]">Propose a circuit to populate it; no DAG is synthesized from a template.</p>
        </div>
      </div>
    );
  }

  return (
    <div data-testid="proof-dag" data-layout-engine={graph.engine} className="absolute inset-0 flex flex-col">
      <div className="pt-20 pb-1 flex items-center justify-center gap-2 font-code text-[12px] text-[var(--color-muted)] shrink-0">
        <ProvenanceBadge kind={proposedOnly ? 'proposed' : 'measured'} detail={proposedOnly ? 'Synthesized proposal; the execution result is in Measurement/Claim.' : 'Executed circuit evidence.'} />
        <span>{dagLabel} · {graph.nodes.length} nodes · {graph.edges.length} edges · {campaign?.target_system || 'untargeted'}</span>
      </div>
      {/* Wire-cut ledger: proposed cuts vs measured cut jobs. Proposed cuts
          are labeled, never presented as executed. */}
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
                  className="px-2 py-0.5 rounded-full font-code text-[12px] border bg-[var(--color-success-wash)] border-[var(--color-success-edge)] text-[var(--color-success-ink)]"
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
                title="Proposed cut — not executed"
                className="px-2 py-0.5 rounded-full font-code text-[12px] border bg-[var(--color-warning-wash)] border-[var(--color-warning-edge)] text-[var(--color-warning-ink)]"
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
            className="px-2 py-0.5 rounded-full font-code text-[12px] border bg-[var(--color-danger-wash)] border-[var(--color-danger-edge)] text-[var(--color-danger-ink)]"
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
              className={`px-2 py-0.5 rounded-full font-code text-[12px] border ${
                s.tone === 'amber'
                  ? 'bg-[var(--color-warning-wash)] border-[var(--color-warning-edge)] text-[var(--color-warning-ink)]'
                  : 'bg-[var(--color-danger-wash)] border-[var(--color-danger-edge)] text-[var(--color-danger-ink)]'
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
          aria-label={`Circuit DAG: ${graph.nodes.length} nodes, ${graph.edges.length} edges`}
        >
          <defs>
            <linearGradient id="dagEdge" x1="0%" y1="0%" x2="100%" y2="0%">
              <stop offset="0%" stopColor="var(--color-info)" stopOpacity="0.65" />
              <stop offset="100%" stopColor="var(--color-info)" stopOpacity="0.18" />
            </linearGradient>
          </defs>
          {graph.clusters.map((cluster) => (
            <g key={cluster.id} className="k-dag-cluster" data-testid="dag-cluster">
              <rect
                x={cluster.x}
                y={cluster.y}
                width={cluster.width}
                height={cluster.height}
                rx={12}
                fill="none"
                stroke="var(--color-border-strong)"
                strokeWidth={1.25}
                strokeDasharray="5 4"
              />
              <text x={cluster.x + 12} y={cluster.y + 15} className="fill-[var(--color-muted)]" fontSize="10" fontFamily="monospace">
                {cluster.label}
              </text>
            </g>
          ))}
          {graph.edges.map((e) => {
            const pathD = graph.edgePaths[e.id];
            if (pathD) {
              return (
                <path
                  key={e.id}
                  d={pathD}
                  fill="none"
                  stroke="url(#dagEdge)"
                  strokeWidth="1.5"
                />
              );
            }
            const a = graph.pos[e.source];
            const b = graph.pos[e.target];
            if (!a || !b) return null;
            const x1 = a.x + DAG_NODE_W;
            const y1 = a.y + DAG_NODE_H / 2;
            const x2 = b.x;
            const y2 = b.y + DAG_NODE_H / 2;
            const mx = (x1 + x2) / 2;
            return (
              <path
                key={e.id}
                d={`M ${x1} ${y1} L ${mx} ${y1} L ${mx} ${y2} L ${x2} ${y2}`}
                fill="none"
                stroke="url(#dagEdge)"
                strokeWidth="1.5"
              />
            );
          })}
          {graph.nodes.map((n) => {
            const p = graph.pos[n.id];
            if (!p) return null;
            const isSel = selectedId === n.id;
            return (
              <g
                key={n.id}
                transform={`translate(${p.x}, ${p.y})`}
                onClick={() => setSelectedId(isSel ? null : n.id)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    setSelectedId(isSel ? null : n.id);
                  }
                }}
                role="button"
                tabIndex={0}
                aria-pressed={isSel}
                aria-label={`Select circuit node ${n.id}`}
                className="cursor-pointer"
              >
                <rect
                  width={DAG_NODE_W}
                  height={DAG_NODE_H}
                  rx="10"
                  className={isSel ? 'fill-[var(--color-accent-soft)] stroke-[var(--color-accent-edge)]' : 'fill-[var(--color-surface)] stroke-[var(--color-border-strong)]'}
                  strokeWidth="1"
                />
                <text x="12" y="15" className="fill-[var(--color-text)]" fontSize="10" fontFamily="monospace">
                  {n.label.length > 26 ? `${n.label.slice(0, 25)}…` : n.label}
                </text>
                <text x="12" y="28" className="fill-[var(--color-muted)]" fontSize="9" fontFamily="monospace">
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
          <div className="max-w-2xl mx-auto p-3.5 rounded-2xl bg-[var(--color-surface-elev)] border border-[var(--color-info-edge)] space-y-1.5 shadow-[var(--shadow-card)]">
            <div className="text-xs font-code text-[var(--color-info-ink)]">{selected.label}</div>
            <div className="text-[12px] font-code text-[var(--color-muted)]">
              id {selected.id} · {selected.kind}
              {typeof selected.angle === 'number' && Number.isFinite(selected.angle) ? ` · θ=${selected.angle}` : ''}
            </div>
            <div className="text-[12px] font-code text-[var(--color-faint)]">
              {provenance ? `ledger receipt ${provenance}` : 'no provenance receipt on this campaign'}
            </div>
          </div>
        ) : (
          <p className="text-center text-[12px] font-code text-[var(--color-faint)]">
            Select a node to trace its record.
          </p>
        )}
        {/* Compile receipt: original QASM vs backend-transpiled QIR. */}
        <div className="max-w-2xl mx-auto mt-2 rounded-2xl bg-[var(--color-surface-elev)] border border-[var(--color-border)] shadow-[var(--shadow-card)]">
          <button
            onClick={() => setCompileOpen((v) => !v)}
            aria-expanded={compileOpen}
            className="w-full flex items-center justify-between px-3.5 py-2 text-[12px] font-code text-[var(--color-label-secondary)] hover:text-[var(--color-text)] transition-colors"
          >
            <span>Compile receipt (QASM → QIR via backend)</span>
            <span>{compileOpen ? '−' : '+'}</span>
          </button>
          {compileOpen && (
            <div className="px-3.5 pb-3.5 space-y-2">
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <div className="text-[12px] font-code text-[var(--color-faint)] mb-1">ORIGINAL QASM (campaign)</div>
                  <pre className="max-h-40 overflow-auto text-[12px] font-code text-[var(--color-label-secondary)] bg-[var(--color-surface)] border border-[var(--color-border)] rounded-lg p-2 whitespace-pre-wrap">
                    {campaign?.quantum_factors?.qasm || 'no circuit in this campaign'}
                  </pre>
                </div>
                <div>
                  <div className="text-[12px] font-code text-[var(--color-faint)] mb-1">TRANSPILED QIR (backend)</div>
                  {!receipt && !compileError && (
                    <button
                      onClick={() => void runCompile()}
                      disabled={compiling || !campaign?.quantum_factors?.qasm}
                      className="px-2.5 py-1.5 rounded-lg bg-[var(--color-info-wash)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-info-edge)] text-[var(--color-info-ink)] text-[12px] transition-all disabled:opacity-40"
                    >
                      {compiling ? 'Compiling…' : 'Compile via backend'}
                    </button>
                  )}
                  {receipt === null && !compileError && (
                    <p className="text-[12px] font-code text-[var(--color-danger-ink)]">Compile refused — nothing transpiled.</p>
                  )}
                  {compileError && (
                    <p className="text-[12px] font-code text-[var(--color-warning-ink)]">
                      Compile transport failed — bridge unreachable, not a refusal ({compileError.slice(0, 160)}). Nothing transpiled.
                    </p>
                  )}
                  {receipt && (
                    <div className="space-y-1.5">
                      <div className="text-[12px] font-code text-[var(--color-muted)]">
                        {receipt.compiler_used || 'unanalyzed'}
                        {receipt.fallback_estimate ? ' · ESTIMATE (not compiler-verified)' : ' · compiler-verified'}
                        {' · '}qubits {receipt.num_qubits}{receipt.fallback_estimate ? ' (EST)' : ''} · depth {receipt.depth}{receipt.fallback_estimate ? ' (EST)' : ''} · gates {receipt.gate_count}{receipt.fallback_estimate ? ' (EST)' : ''} · T {receipt.t_count}{receipt.fallback_estimate ? ' (EST)' : ''}
                      </div>
                      <pre className="max-h-40 overflow-auto text-[12px] font-code text-[var(--color-label-secondary)] bg-[var(--color-surface)] border border-[var(--color-border)] rounded-lg p-2 whitespace-pre-wrap">
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
