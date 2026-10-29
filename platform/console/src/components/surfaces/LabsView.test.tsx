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
  wasRecentAuthFailure: vi.fn(() => false),
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
    vi.mocked(api.wasRecentAuthFailure).mockReturnValue(false);
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
    vi.mocked(api.authWhoami).mockResolvedValue({ authenticated: true });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LabsView />);
    });
    expect(textOf(root!.root)).toContain('No laboratories registered.');
  });

  it('renders a login prompt (not a fake empty) when labs are refused while logged out', async () => {
    vi.mocked(api.fetchLabs).mockResolvedValue([]);
    vi.mocked(api.fetchSessions).mockResolvedValue([]);
    vi.mocked(api.authWhoami).mockResolvedValue({ authenticated: false });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LabsView />);
    });
    const text = textOf(root!.root);
    expect(text).toContain('requires login');
    expect(text).toContain('Operator session');
    expect(text).not.toContain('No laboratories registered.');
  });

  it('treats a recent 401 as refused even before whoami resolves', async () => {
    vi.mocked(api.fetchLabs).mockResolvedValue([]);
    vi.mocked(api.fetchSessions).mockResolvedValue([]);
    vi.mocked(api.authWhoami).mockResolvedValue({ authenticated: true });
    vi.mocked(api.wasRecentAuthFailure).mockReturnValue(true);
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LabsView />);
    });
    expect(textOf(root!.root)).toContain('requires login');
  });
});

describe('LabsView money precision + spend bar (Wave-2)', () => {
  beforeEach(() => {
    vi.mocked(api.fetchLabs).mockResolvedValue([
      { id: 'lab-a', display_name: 'Lab A', autonomy_level: 'L1', spend_ceiling_usd: 99.9, spent_usd: 10.5, status: 'active' },
    ]);
    vi.mocked(api.fetchSessions).mockResolvedValue([
      { id: 's1', title: 'drill', tenant_id: 'lab-a', created_at: '', updated_at: '', status: 'active', spend_limit_usd: 120.126, current_spend_usd: 3.14159 },
    ]);
    vi.mocked(api.authWhoami).mockResolvedValue({ authenticated: true });
    vi.mocked(api.wasRecentAuthFailure).mockReturnValue(false);
  });

  it('renders all money at 2dp', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LabsView />);
    });
    const text = textOf(root!.root);
    expect(text).toContain('$10.50 / $99.90');
    await act(async () => {
      root!.root.findAllByType('button').filter((b) => b.props['aria-expanded'] !== undefined)[0].props.onClick();
    });
    expect(textOf(root!.root)).toContain('$3.14 / $120.13');
  });

  it('exposes the spend bar as a progressbar with value, range, and name', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LabsView />);
    });
    const bar = root!.root.findByProps({ role: 'progressbar' });
    expect(bar.props['aria-valuemin']).toBe(0);
    expect(bar.props['aria-valuemax']).toBe(100);
    // 10.5 / 99.9 -> 11%.
    expect(bar.props['aria-valuenow']).toBe(11);
    expect(String(bar.props['aria-label'])).toMatch(/Lab A/);
  });

  it('names an unbounded bar without inventing a ceiling', async () => {
    vi.mocked(api.fetchLabs).mockResolvedValue([
      { id: 'lab-x', display_name: 'Lab X', autonomy_level: 'L1', spend_ceiling_usd: 0, spent_usd: 5, status: 'active' },
    ]);
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LabsView />);
    });
    const bar = root!.root.findByProps({ role: 'progressbar' });
    expect(String(bar.props['aria-label'])).toMatch(/Lab X/);
    expect(String(bar.props['aria-valuetext'])).toMatch(/unbounded/);
    expect(textOf(root!.root)).toContain('UNBOUNDED');
  });
});
