import type { ElkExtendedEdge, ElkNode } from 'elkjs/lib/elk-api';
import { asRecord } from '@/lib/guards';

export interface DagNode {
  id: string;
  label: string;
  kind: string;
  angle?: number;
  fragment?: number;
}

export interface DagEdge {
  id: string;
  source: string;
  target: string;
}

export interface DagCluster {
  id: string;
  label: string;
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface DagLayoutResult {
  nodes: DagNode[];
  edges: DagEdge[];
  pos: Record<string, { x: number; y: number }>;
  edgePaths: Record<string, string>;
  clusters: DagCluster[];
  width: number;
  height: number;
  hasCycle: boolean;
  engine: 'sync-fallback' | 'elk-worker';
}

export const DAG_NODE_W = 190;
export const DAG_NODE_H = 36;

function asStr(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

export function parseDagInputs(
  rawNodes: readonly unknown[],
  rawEdges: readonly unknown[],
): { nodes: DagNode[]; edges: DagEdge[] } {
  const nodes: DagNode[] = rawNodes.map((n, i) => {
    const r = asRecord(n);
    const data = asRecord(r.data);
    const id = asStr(r.id) || `node-${i}`;
    const label = asStr(data.label) || asStr(r.label) || asStr(r.op) || asStr(r.name) || id;
    const kind = asStr(data.type) || asStr(r.type) || asStr(r.kind) || asStr(r.gate) || 'node';
    const angle = typeof data.angle === 'number' ? data.angle : typeof r.angle === 'number' ? r.angle : undefined;
    const rawFrag = data.fragment ?? r.fragment;
    const fragment = typeof rawFrag === 'number' && Number.isFinite(rawFrag)
      ? Math.max(0, Math.floor(rawFrag))
      : undefined;
    return { id, label, kind, angle, fragment };
  });
  const ids = new Set(nodes.map((n) => n.id));
  const edges: DagEdge[] = rawEdges
    .map((e, i) => {
      const r = asRecord(e);
      const source = asStr(r.source) || asStr(r.from);
      const target = asStr(r.target) || asStr(r.to);
      return { id: asStr(r.id) || `e-${i}`, source, target };
    })
    .filter((e) => e.source && e.target && ids.has(e.source) && ids.has(e.target));
  return { nodes, edges };
}

export function partitionNodesByWireCuts(
  nodes: readonly DagNode[],
  wireCuts: readonly Record<string, unknown>[],
): Map<string, number> {
  const out = new Map<string, number>();
  if (nodes.length === 0 || wireCuts.length === 0) return out;
  const fragmentCount = Math.min(nodes.length, wireCuts.length + 1);
  if (fragmentCount < 2) return out;
  const chunkSize = Math.ceil(nodes.length / fragmentCount);
  nodes.forEach((node, idx) => {
    const frag = typeof node.fragment === 'number'
      ? Math.min(fragmentCount - 1, node.fragment)
      : Math.min(fragmentCount - 1, Math.floor(idx / chunkSize));
    out.set(node.id, frag);
  });
  return out;
}

export function orthogonalSectionPath(points: readonly { x: number; y: number }[]): string {
  if (points.length === 0) return '';
  let d = `M ${Math.round(points[0]!.x * 10) / 10} ${Math.round(points[0]!.y * 10) / 10}`;
  for (let i = 1; i < points.length; i += 1) {
    d += ` L ${Math.round(points[i]!.x * 10) / 10} ${Math.round(points[i]!.y * 10) / 10}`;
  }
  return d;
}

export function buildFallbackDagLayout(
  rawNodes: readonly unknown[],
  rawEdges: readonly unknown[],
  wireCuts: readonly Record<string, unknown>[] = [],
): DagLayoutResult {
  const { nodes, edges } = parseDagInputs(rawNodes, rawEdges);
  const inDeg = new Map<string, number>(nodes.map((n) => [n.id, 0]));
  const out = new Map<string, string[]>();
  for (const e of edges) {
    inDeg.set(e.target, (inDeg.get(e.target) ?? 0) + 1);
    const list = out.get(e.source) ?? [];
    list.push(e.target);
    out.set(e.source, list);
  }
  const layer = new Map<string, number>();
  const queue = nodes.filter((n) => (inDeg.get(n.id) ?? 0) === 0).map((n) => n.id);
  const hasCycleSeed = queue.length === 0 && nodes.length > 0 && edges.length > 0;
  if (queue.length === 0 && nodes.length > 0) queue.push(nodes[0]!.id);
  for (const id of queue) layer.set(id, 0);
  const order = [...queue];
  for (let h = 0; h < order.length; h += 1) {
    const id = order[h]!;
    for (const next of out.get(id) ?? []) {
      const cand = (layer.get(id) ?? 0) + 1;
      if (cand > (layer.get(next) ?? -1)) layer.set(next, cand);
      if (!order.includes(next)) order.push(next);
    }
  }
  for (const n of nodes) {
    if (!layer.has(n.id)) layer.set(n.id, 0);
  }
  const hasCycle = hasCycleSeed || order.length < nodes.length;
  const columns = new Map<number, DagNode[]>();
  let maxLayer = 0;
  for (const n of nodes) {
    const l = layer.get(n.id) ?? 0;
    maxLayer = Math.max(maxLayer, l);
    if (!columns.has(l)) columns.set(l, []);
    columns.get(l)!.push(n);
  }
  const COL_X = 240;
  const ROW_Y = 66;
  const PAD_X = 28;
  const PAD_Y = 36;
  const pos: Record<string, { x: number; y: number }> = {};
  for (const [l, list] of columns.entries()) {
    list.forEach((n, row) => {
      pos[n.id] = { x: PAD_X + l * COL_X, y: PAD_Y + row * ROW_Y };
    });
  }

  const edgePaths: Record<string, string> = {};
  for (const e of edges) {
    const a = pos[e.source];
    const b = pos[e.target];
    if (!a || !b) continue;
    const x1 = a.x + DAG_NODE_W;
    const y1 = a.y + DAG_NODE_H / 2;
    const x2 = b.x;
    const y2 = b.y + DAG_NODE_H / 2;
    const mid = x1 + (x2 - x1) / 2;
    edgePaths[e.id] = Math.abs(y1 - y2) < 1
      ? `M ${x1} ${y1} L ${x2} ${y2}`
      : `M ${x1} ${y1} L ${mid} ${y1} L ${mid} ${y2} L ${x2} ${y2}`;
  }

  const fragMap = partitionNodesByWireCuts(nodes, wireCuts);
  const clusters = buildClustersFromPositions(nodes, pos, fragMap);
  const maxRows = columns.size > 0 ? Math.max(...[...columns.values()].map((c) => c.length)) : 1;
  return {
    nodes,
    edges,
    pos,
    edgePaths,
    clusters,
    width: PAD_X + maxLayer * COL_X + DAG_NODE_W + 40,
    height: Math.max(140, PAD_Y + maxRows * ROW_Y + 24),
    hasCycle,
    engine: 'sync-fallback',
  };
}

function buildClustersFromPositions(
  nodes: readonly DagNode[],
  pos: Record<string, { x: number; y: number }>,
  fragMap: Map<string, number>,
): DagCluster[] {
  if (fragMap.size === 0) return [];
  const bounds = new Map<number, { minX: number; minY: number; maxX: number; maxY: number }>();
  for (const n of nodes) {
    const frag = fragMap.get(n.id);
    const p = pos[n.id];
    if (frag === undefined || !p) continue;
    const cur = bounds.get(frag);
    if (!cur) {
      bounds.set(frag, { minX: p.x, minY: p.y, maxX: p.x + DAG_NODE_W, maxY: p.y + DAG_NODE_H });
    } else {
      cur.minX = Math.min(cur.minX, p.x);
      cur.minY = Math.min(cur.minY, p.y);
      cur.maxX = Math.max(cur.maxX, p.x + DAG_NODE_W);
      cur.maxY = Math.max(cur.maxY, p.y + DAG_NODE_H);
    }
  }
  return Array.from(bounds.entries())
    .sort((a, b) => a[0] - b[0])
    .map(([frag, b]) => ({
      id: `fragment-${frag}`,
      label: `Partition ${frag + 1}`,
      x: b.minX - 14,
      y: b.minY - 24,
      width: b.maxX - b.minX + 28,
      height: b.maxY - b.minY + 38,
    }));
}

export function buildElkGraphInput(
  nodes: readonly DagNode[],
  edges: readonly DagEdge[],
  wireCuts: readonly Record<string, unknown>[] = [],
): ElkNode {
  const fragMap = partitionNodesByWireCuts(nodes, wireCuts);
  const rootEdges: ElkExtendedEdge[] = edges.map((e) => ({
    id: e.id,
    sources: [e.source],
    targets: [e.target],
  }));

  if (fragMap.size === 0) {
    return {
      id: 'proof-dag-root',
      layoutOptions: {
        'elk.algorithm': 'layered',
        'elk.direction': 'RIGHT',
        'elk.edgeRouting': 'ORTHOGONAL',
        'elk.spacing.nodeNode': '24',
        'elk.layered.spacing.nodeNodeBetweenLayers': '52',
        'elk.padding': '[top=24,left=24,bottom=24,right=24]',
      },
      children: nodes.map((n) => ({ id: n.id, width: DAG_NODE_W, height: DAG_NODE_H })),
      edges: rootEdges,
    };
  }

  const grouped = new Map<number, DagNode[]>();
  for (const n of nodes) {
    const frag = fragMap.get(n.id) ?? 0;
    const list = grouped.get(frag);
    if (list) list.push(n);
    else grouped.set(frag, [n]);
  }

  const containers: ElkNode[] = Array.from(grouped.entries())
    .sort((a, b) => a[0] - b[0])
    .map(([frag, members]) => ({
      id: `fragment-${frag}`,
      labels: [{ text: `Partition ${frag + 1}`, width: 96, height: 18 }],
      layoutOptions: {
        'elk.padding': '[top=30,left=16,bottom=16,right=16]',
        'elk.spacing.nodeNode': '20',
        'elk.layered.spacing.nodeNodeBetweenLayers': '42',
      },
      children: members.map((n) => ({ id: n.id, width: DAG_NODE_W, height: DAG_NODE_H })),
    }));

  return {
    id: 'proof-dag-root',
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': 'RIGHT',
      'elk.edgeRouting': 'ORTHOGONAL',
      'org.eclipse.elk.hierarchyHandling': 'INCLUDE_CHILDREN',
      'elk.spacing.nodeNode': '28',
      'elk.layered.spacing.nodeNodeBetweenLayers': '56',
      'elk.padding': '[top=24,left=24,bottom=24,right=24]',
    },
    children: containers,
    edges: rootEdges,
  };
}

export function extractElkLayoutResult(
  root: ElkNode,
  nodes: readonly DagNode[],
  edges: readonly DagEdge[],
  hasCycle: boolean,
): DagLayoutResult {
  const pos: Record<string, { x: number; y: number }> = {};
  const clusters: DagCluster[] = [];
  const edgePaths: Record<string, string> = {};

  const walkNode = (node: ElkNode) => {
    if (node.id.startsWith('fragment-') && typeof node.x === 'number' && typeof node.y === 'number') {
      const idx = Number(node.id.replace('fragment-', ''));
      clusters.push({
        id: node.id,
        label: Number.isFinite(idx) ? `Partition ${idx + 1}` : node.id,
        x: node.x,
        y: node.y,
        width: node.width ?? DAG_NODE_W + 32,
        height: node.height ?? DAG_NODE_H + 46,
      });
    } else if (node.id !== 'proof-dag-root' && typeof node.x === 'number' && typeof node.y === 'number') {
      pos[node.id] = { x: node.x, y: node.y };
    }
    for (const edge of node.edges ?? []) {
      const sec = (edge as ElkExtendedEdge).sections?.[0];
      if (!sec) continue;
      edgePaths[edge.id] = orthogonalSectionPath([
        sec.startPoint,
        ...(sec.bendPoints ?? []),
        sec.endPoint,
      ]);
    }
    for (const child of node.children ?? []) {
      walkNode(child);
    }
  };

  walkNode(root);
  return {
    nodes: [...nodes],
    edges: [...edges],
    pos,
    edgePaths,
    clusters,
    width: Math.max(320, Math.ceil((root.width ?? 400) + 24)),
    height: Math.max(140, Math.ceil((root.height ?? 160) + 24)),
    hasCycle,
    engine: 'elk-worker',
  };
}
