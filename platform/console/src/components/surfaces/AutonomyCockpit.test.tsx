import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { AutonomyCockpit } from './AutonomyCockpit';
import { ApprovalPacketCard } from '@/components/common/ApprovalPacketCard';
import type { CampaignState, Session } from '@/types';

vi.mock('@/lib/authorizeSpend', () => ({
  authorizeSpend: vi.fn(),
}));

import { authorizeSpend } from '@/lib/authorizeSpend';

const SESSION: Session = {
  id: 'sess-1',
  title: 't',
  tenant_id: 'lab-alpha',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  status: 'active',
  spend_limit_usd: 150,
  current_spend_usd: 0,
};

function campaign(): CampaignState {
  return {
    session_id: 'sess-1',
    target_system: 'H2',
    quantum_factors: {
      estimated_spend_usd: 12.5,
      qasm: 'OPENQASM 2.0;\nqreg q[1];\n',
      recommended_backend: 'ibm_brisbane',
    },
  } as unknown as CampaignState;
}

function instanceText(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(instanceText).join(' ');
  if (node && typeof node === 'object') {
    const rec = node as { children?: unknown; props?: { children?: unknown } };
    // Test instances expose .children; raw React elements expose
    // .props.children. Never JSON.stringify — fibers are circular.
    if (Array.isArray(rec.children)) return rec.children.map(instanceText).join(' ');
    if (rec.props && 'children' in rec.props) return instanceText(rec.props.children);
  }
  return '';
}

interface Scenario {
  airGapped: boolean | null;
  reachable: boolean;
  ready: boolean;
  readyMessage?: string;
}

function stubBackend(s: Scenario) {
  vi.stubGlobal(
    'fetch',
    async (url: string) => {
      const u = String(url);
      if (u.includes('/api/llm/settings')) {
        if (s.airGapped === null) throw new Error('settings unreachable');
        return { ok: true, json: async () => ({ air_gapped: s.airGapped }) };
      }
      if (u.includes('/api/health')) {
        if (!s.reachable) throw new Error('connection refused');
        return { ok: true, json: async () => ({ service: 'kubonad-test', version: '0.0.0' }) };
      }
      if (u.includes('/api/ready')) {
        if (!s.reachable) throw new Error('connection refused');
        if (!s.ready) {
          return { ok: false, status: 503, json: async () => ({ message: s.readyMessage || 'not ready' }) };
        }
        return { ok: true, json: async () => ({ ready: true }) };
      }
      if (u.includes('/api/metasearch/proposals')) {
        return { ok: true, json: async () => ({ proposals: [] }) };
      }
      return { ok: false, status: 404, json: async () => ({}) };
    }
  );
}

async function renderCockpit(session: Session | null = SESSION) {
  let tree: TestRenderer.ReactTestRenderer;
  await act(async () => {
    tree = TestRenderer.create(<AutonomyCockpit campaign={campaign()} session={session} />);
  });
  return tree!;
}

function badgeText(tree: TestRenderer.ReactTestRenderer): string {
  const badge = tree.root.findByProps({ 'data-testid': 'sentinel-badge' });
  return instanceText(badge);
}

beforeEach(() => {
  vi.unstubAllGlobals();
  vi.mocked(authorizeSpend).mockReset();
});

describe('AutonomyCockpit Sentinel badge (live predicate)', () => {
  it('claims Protection Active only when backend is ready AND air-gap is engaged', async () => {
    stubBackend({ airGapped: true, reachable: true, ready: true });
    const text = badgeText(await renderCockpit());
    expect(text).toContain('Sentinel Protection Active');
    expect(text).not.toContain('degraded');
    expect(text).not.toContain('controls enabled');
  });

  it('renders degraded (never Active) when the air-gap is released', async () => {
    stubBackend({ airGapped: false, reachable: true, ready: true });
    const text = badgeText(await renderCockpit());
    expect(text).toContain('Sentinel degraded');
    expect(text).toContain('air-gap released');
    expect(text).not.toContain('Protection Active');
  });

  it('renders degraded with the backend reason when readiness 503s', async () => {
    stubBackend({ airGapped: true, reachable: true, ready: false, readyMessage: 'disk blocked' });
    const text = badgeText(await renderCockpit());
    expect(text).toContain('Sentinel degraded');
    expect(text).toContain('disk blocked');
    expect(text).not.toContain('Protection Active');
  });

  it('demotes to neutral controls copy when predicates are unreachable', async () => {
    stubBackend({ airGapped: null, reachable: false, ready: false });
    const text = badgeText(await renderCockpit());
    expect(text).toContain('Sentinel controls enabled');
    expect(text).not.toContain('Protection Active');
    // No static assurance chrome remains anywhere on the surface.
    expect(text).not.toContain('Envelope Verified');
  });
});

describe('AutonomyCockpit envelope line (per-dispatch receipt)', () => {
  function openAttestation(tree: TestRenderer.ReactTestRenderer) {
    const inspect = tree.root
      .findAllByType('button')
      .find((b) => instanceText(b).includes('Inspect Cryptographic'));
    expect(inspect).toBeDefined();
    act(() => {
      inspect!.props.onClick();
    });
  }

  it('states no envelope verified yet (never a static verified claim)', async () => {
    stubBackend({ airGapped: true, reachable: true, ready: true });
    const tenantless = { ...SESSION, tenant_id: undefined };
    const tree = await renderCockpit(tenantless);
    openAttestation(tree);
    const json = JSON.stringify(tree.toJSON());
    expect(json).toContain('No envelope verified yet');
    // Serialized children split the label ("Tenant: ","unbound",...).
    expect(json).toContain('Tenant: ');
    expect(json).toContain('unbound');
    expect(json).not.toContain('Envelope Verified Statelessly');
  });

  it('derives the envelope line from the verified dispatch receipt', async () => {
    stubBackend({ airGapped: true, reachable: true, ready: true });
    vi.mocked(authorizeSpend).mockResolvedValue({
      ok: true,
      mandate: {
        tenant_id: 'lab-alpha',
        job_hash: 'jh',
        max_budget_usd: 12.5,
        expiry_timestamp: '2026-01-01T01:00:00Z',
        public_key_hex: 'pk',
        signature_hex: 'ab12cd34ef56ab12cd34ef56ab12cd34',
      },
      tenantId: 'lab-alpha',
      jobHash: 'jh',
    });
    const tree = await renderCockpit();
    const card = tree.root.findByType(ApprovalPacketCard);
    await act(async () => {
      await card.props.onApproveOnce();
    });
    openAttestation(tree);
    const json = JSON.stringify(tree.toJSON());
    expect(json).toContain('Envelope verified');
    expect(json).toContain('ab12cd34ef56ab12');
    expect(json).toContain('lab-alpha');
    expect(json).not.toContain('Envelope Verified Statelessly');
  });
});

describe('AutonomyCockpit Wave-2 money copy + toggle staging', () => {
  function stubSettingsAirGap(airGapped: boolean, onSave?: (init?: RequestInit) => unknown) {
    vi.stubGlobal(
      'fetch',
      async (url: string, init?: RequestInit) => {
        const u = String(url);
        if (u.includes('/api/llm/settings')) {
          if ((init as RequestInit | undefined)?.method === 'POST') {
            return (onSave?.(init) ?? { ok: true, json: async () => ({}) }) as Response;
          }
          return { ok: true, json: async () => ({ air_gapped: airGapped }) } as unknown as Response;
        }
        if (u.includes('/api/health')) {
          return { ok: true, json: async () => ({ service: 'kubonad-test', version: '0.0.0' }) } as unknown as Response;
        }
        if (u.includes('/api/ready')) {
          return { ok: true, json: async () => ({ ready: true }) } as unknown as Response;
        }
        if (u.includes('/api/metasearch/proposals')) {
          return { ok: true, json: async () => ({ proposals: [] }) } as unknown as Response;
        }
        return { ok: false, status: 404, json: async () => ({}) } as unknown as Response;
      }
    );
  }

  function airGapToggle(tree: TestRenderer.ReactTestRenderer) {
    return tree.root
      .findAllByType('button')
      .find((b) => String(b.props.title ?? '').includes('Persists to intelligence'))!;
  }

  function fullText(tree: TestRenderer.ReactTestRenderer): string {
    return JSON.stringify(tree.toJSON());
  }

  it('Simulate Offline states the in-scope $0.00 amount (2dp)', async () => {
    stubBackend({ airGapped: true, reachable: true, ready: true });
    const tree = await renderCockpit();
    const offline = tree.root
      .findAllByType('button')
      .find((b) => instanceText(b).includes('Simulate Offline'))!;
    expect(instanceText(offline)).toContain('Simulate Offline — $0.00');
  });

  it('Air-Gap toggle persists immediately with revert-on-failure: success reports persisted', async () => {
    const saved: string[] = [];
    stubSettingsAirGap(true, (init) => {
      saved.push(String(init?.body));
      return { ok: true, json: async () => ({}) };
    });
    const tree = await renderCockpit();
    await vi.waitFor(() => expect(airGapToggle(tree).props.disabled).toBe(false));
    await act(async () => {
      await airGapToggle(tree).props.onClick();
    });
    await vi.waitFor(() => expect(fullText(tree)).toContain('released (persisted)'));
    expect(saved).toHaveLength(1);
    expect(JSON.parse(saved[0]).air_gapped).toBe(false);
  });

  it('Air-Gap refusal reverts the switch with a reverted banner (nothing silently kept)', async () => {
    stubSettingsAirGap(true, () => ({ ok: false, status: 500, text: async () => 'disk blocked' }));
    const tree = await renderCockpit();
    await vi.waitFor(() => expect(airGapToggle(tree).props.disabled).toBe(false));
    await act(async () => {
      await airGapToggle(tree).props.onClick();
    });
    await vi.waitFor(() => expect(fullText(tree)).toContain('reverted'));
    expect(fullText(tree)).toContain('disk blocked');
    // Reverted: the badge still derives the engaged predicate (no silent flip).
    expect(badgeText(tree)).toContain('Sentinel Protection Active');
  });
});
