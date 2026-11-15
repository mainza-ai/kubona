'use client';

import React from 'react';
import { AlertTriangle, ShieldCheck } from 'lucide-react';
import type { CitationClaim, CitationReport } from '@/types';

function statusLabel(status: string): string {
  switch (status) {
    case 'uncited':
      return 'no citation';
    case 'unknown_observation':
      return 'unknown observation id';
    case 'not_in_observation':
      return 'number not in that observation';
    default:
      return status;
  }
}

/**
 * B5 — one badge per reply: how many numeric claims cite a matching
 * observation. Unverified claims are named in the title, never hidden.
 */
export function CitationBadge({ report }: { report?: CitationReport | null }) {
  if (!report || !Array.isArray(report.claims) || report.claims.length === 0) return null;
  const total = report.claims.length;
  const unverified: CitationClaim[] = report.claims.filter((claim) => claim.status !== 'verified');
  const details = unverified
    .slice(0, 8)
    .map((claim) => `${claim.number}${claim.citation ? ` [${claim.citation}]` : ''} — ${statusLabel(claim.status)}`)
    .join('\n');
  const tone = unverified.length > 0 ? 'partial' : 'verified';
  return (
    <div
      data-testid="citation-badge"
      data-state={tone}
      role="status"
      title={details || 'Every numeric claim cites a matching observation.'}
      className={`mt-1 inline-flex max-w-full items-center gap-1.5 rounded-control-compact border px-2 py-1 font-code text-[11px] ${
        unverified.length > 0
          ? 'border-warning-edge bg-warning-wash text-warning-ink'
          : 'border-success-edge bg-success-wash text-success-ink'
      }`}
    >
      {unverified.length > 0 ? <AlertTriangle className="h-3 w-3" /> : <ShieldCheck className="h-3 w-3" />}
      <span>{`${report.verified}/${total} numeric claims verified${unverified.length > 0 ? ` · ${unverified.length} unverified` : ''}`}</span>
    </div>
  );
}
