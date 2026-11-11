/// <reference lib="webworker" />

import ELK from 'elkjs/lib/elk.bundled.js';
import {
  buildElkGraphInput,
  extractElkLayoutResult,
  type DagEdge,
  type DagLayoutResult,
  type DagNode,
} from './proof-dag-layout';

export type ProofDagWorkerRequest = {
  seq: number;
  nodes: DagNode[];
  edges: DagEdge[];
  wireCuts: Record<string, unknown>[];
  hasCycle: boolean;
};

export type ProofDagWorkerResponse =
  | { seq: number; layout: DagLayoutResult }
  | { seq: number; error: string };

const elk = new ELK();

self.onmessage = async (event: MessageEvent<ProofDagWorkerRequest>) => {
  const { seq, nodes, edges, wireCuts, hasCycle } = event.data;
  try {
    const graphInput = buildElkGraphInput(nodes, edges, wireCuts);
    const laidOut = await elk.layout(graphInput, {
      layoutOptions: {
        'json.shapeCoords': 'ROOT',
        'json.edgeCoords': 'ROOT',
      },
    });
    const layout = extractElkLayoutResult(laidOut, nodes, edges, hasCycle);
    const payload: ProofDagWorkerResponse = { seq, layout };
    self.postMessage(payload);
  } catch (err) {
    const payload: ProofDagWorkerResponse = {
      seq,
      error: err instanceof Error ? err.message : String(err),
    };
    self.postMessage(payload);
  }
};
