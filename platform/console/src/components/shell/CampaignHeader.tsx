'use client';

import React from 'react';
import { ChevronDown } from 'lucide-react';
import type { CampaignState, Session } from '@/types';

export function CampaignHeader({
  campaign,
  session,
  statusText,
  mode,
  onOpenCampaigns,
}: {
  campaign: CampaignState | null;
  session: Session | null;
  statusText?: string | null;
  mode?: 'local_private' | 'multi_lab';
  onOpenCampaigns: () => void;
}) {
  const title = session?.title || 'No campaign selected';
  const target = campaign?.target_system || campaign?.quantum_factors?.target_molecule || 'no target';
  const spend = typeof session?.current_spend_usd === 'number' ? session.current_spend_usd : 0;
  const cap = typeof session?.spend_limit_usd === 'number' ? session.spend_limit_usd : null;
  return (
    <header className="k-campaign-header" data-testid="campaign-header">
      <button type="button" className="k-campaign-name" onClick={onOpenCampaigns}>
        <span className="k-campaign-title">{title}</span>
        <ChevronDown aria-hidden="true" size={14} />
      </button>
      <span className="k-campaign-meta">{target}</span>
      {mode && (
        <span
          className={`k-campaign-meta ${mode === 'local_private' ? 'text-emerald-300' : 'text-amber-300'}`}
          title={mode === 'local_private' ? 'Single-lab local deployment.' : 'Multi-lab deployment: sign in to use lab-scoped chat.'}
        >
          {mode === 'local_private' ? 'Local private' : 'Multi-lab'}
        </span>
      )}
      <span className="k-campaign-meta">{statusText || 'idle'}</span>
      <span className="k-campaign-spend">
        ${spend.toFixed(2)}{cap != null ? ` / $${cap.toFixed(2)}` : ''}
      </span>
      <button type="button" className="k-campaign-account" onClick={onOpenCampaigns}>Campaigns</button>
    </header>
  );
}
