import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { LabsView } from './LabsView';
import * as api from '@/lib/api';

vi.mock('@/lib/api', () => ({
  fetchLabs: vi.fn(),
  fetchSessions: vi.fn(),
  authWhoami: vi.fn(),
  authSessions: vi.fn(),
  authChallenge: vi.fn(),
  authLogin: vi.fn(),
  authLogout: vi.fn(),
  authRevoke: vi.fn(),
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
    vi.mocked(api.authWhoami).mockResolvedValue({ authenticated: false });
    vi.mocked(api.authSessions).mockResolvedValue([]);
  });

  it('renders labs with spend figures, autonomy badges, and the operator session auth panel', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LabsView />);
    });
    const text = textOf(root!.root);
    expect(text).toContain('Lab A');
    expect(text).toContain('Lab B');
    expect(text).toContain('$100.00 / $150.00');
    expect(text).toContain('L1');
    expect(text).toContain('suspended');
    expect(text).toContain('Operator session');
    expect(text).toContain('Log in with browser key');
  });

  it('expands a lab to show its sessions with limits', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LabsView />);
    });
    const labButtons = root!.root.findAllByType('button').filter((b) => b.props['aria-expanded'] !== undefined);
    expect(labButtons.length).toBeGreaterThanOrEqual(2);
    await act(async () => {
      labButtons[0].props.onClick();
    });
    const text = textOf(root!.root);
    expect(text).toContain('drill');
    expect(text).toContain('$70.00 / $120.00');
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
