'use client';

import React, { useMemo, useState } from 'react';
import { GitBranch } from 'lucide-react';
import { CampaignState } from '@/types';

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

function asRecord(v: unknown): Record<string, unknown> {
  return (v && typeof v === 'object' ? v : {}) as Record<string, unknown>;
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

/**
 * Layered proof DAG: nodes laid out by longest-path layer over the REAL
 * edge set (sources left, sinks right), edges drawn as connectors.
 * Clicking a node shows its record + the campaign provenance receipt —
 * every element traces to something, per the provenance rule.
 */
export function ProofDagView({ campaign }: ProofDagViewProps) {
  const [selectedId, setSelectedId] = useState<string | null>(null);

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
    const queue: string[] = nodes.filter((n) => (inDeg.get(n.id) ?? 0) === 0).map((n) => n.id);
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
    return { nodes, edges, pos, width: 30 + maxLayer * COL_X + 220, height: Math.max(120, 30 + maxRows * ROW_Y) };
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
                  {n.angle != null ? ` · θ=${n.angle.toFixed(3)}` : ''}
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
              {selected.angle != null ? ` · θ=${selected.angle}` : ''}
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
      </div>
    </div>
  );
}
