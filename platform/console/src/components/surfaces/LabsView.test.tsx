import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { LabsView } from './LabsView';
import * as api from '@/lib/api';

vi.mock('@/lib/api', () => ({
  fetchLabs: vi.fn(),
  fetchSessions: vi.fn(),
}));

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

describe('LabsView', () => {
  beforeEach(() => {
    vi.mocked(api.fetchLabs).mockResolvedValue([
      { id: 'lab-a', display_name: 'Lab A', autonomy_level: 'L1', spend_ceiling_usd: 150, spent_usd: 100, status: 'active' },
      { id: 'lab-b', display_name: 'Lab B', autonomy_level: 'L2', spend_ceiling_usd: 150, spent_usd: 140, status: 'suspended' },
    ]);
    vi.mocked(api.fetchSessions).mockResolvedValue([
      { id: 's1', title: 'drill', tenant_id: 'lab-a', created_at: '', updated_at: '', status: 'active', spend_limit_usd: 120, current_spend_usd: 70 },
    ]);
  });

  it('renders labs with spend figures and autonomy badges', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LabsView />);
    });
    const text = textOf(root!.root);
    expect(text).toContain('Lab A');
    expect(text).toContain('Lab B');
    expect(text).toContain('$100 / $150');
    expect(text).toContain('L1');
    expect(text).toContain('suspended');
  });

  it('expands a lab to show its sessions with limits', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LabsView />);
    });
    const buttons = root!.root.findAllByType('button');
    await act(async () => {
      buttons[0].props.onClick();
    });
    const text = textOf(root!.root);
    expect(text).toContain('drill');
    expect(text).toContain('$70 / $120');
  });

  it('renders empty state when no labs exist', async () => {
    vi.mocked(api.fetchLabs).mockResolvedValue([]);
    vi.mocked(api.fetchSessions).mockResolvedValue([]);
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LabsView />);
    });
    expect(textOf(root!.root)).toContain('No laboratories registered.');
  });
});
