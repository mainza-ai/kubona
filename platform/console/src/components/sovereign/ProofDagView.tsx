'use client';

import React, { useMemo } from 'react';
import { GitBranch } from 'lucide-react';
import { CampaignState } from '@/types';

interface ProofDagViewProps {
  campaign?: CampaignState | null;
}

function nodeLabel(node: Record<string, unknown>, index: number): string {
  const label = node['label'] ?? node['id'] ?? node['name'];
  return typeof label === 'string' && label ? label : `node-${index}`;
}

export function ProofDagView({ campaign }: ProofDagViewProps) {
  const nodes = useMemo(
    () => campaign?.quantum_factors?.dag_nodes ?? [],
    [campaign?.quantum_factors?.dag_nodes]
  );
  const edges = useMemo(
    () => campaign?.quantum_factors?.dag_edges ?? [],
    [campaign?.quantum_factors?.dag_edges]
  );

  if (nodes.length === 0) {
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
    <div data-testid="proof-dag" className="absolute inset-0 overflow-y-auto">
      <div className="min-h-full flex flex-col items-center justify-center gap-0 py-16 px-6">
        <div className="font-code text-[10px] text-slate-500 mb-4">
          {nodes.length} nodes · {edges.length} edges · {campaign?.target_system || 'untargeted'}
        </div>
        {nodes.map((node, i) => (
          <React.Fragment key={i}>
            {i > 0 && <div className="w-px h-5 bg-gradient-to-b from-cyan-400/60 to-cyan-400/10" />}
            <div className="px-4 py-2 rounded-xl bg-[#0E1117]/90 border border-cyan-400/25 text-xs font-code text-cyan-200 shadow-lg max-w-md truncate">
              {nodeLabel(node, i)}
            </div>
          </React.Fragment>
        ))}
      </div>
    </div>
  );
}
