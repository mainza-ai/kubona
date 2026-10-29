import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { EpistemicTribunalRoom } from './EpistemicTribunalRoom';
import ConfirmDialog from '@/components/common/ConfirmDialog';

afterEach(() => {
  vi.unstubAllGlobals();
});

function turn(role: string, name: string) {
  return { role, name, content: `${role} statement`, verdict: 'pass', turn_index: 1, timestamp: '' };
}

function stubFetch(handler: (url: string, init?: RequestInit) => unknown) {
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => handler(url, init) as Response);
}

async function render(el: React.ReactElement): Promise<TestRenderer.ReactTestRenderer> {
  let tree: TestRenderer.ReactTestRenderer;
  await act(async () => {
    tree = TestRenderer.create(el);
  });
  return tree!;
}

const TURNS = [
  turn('ansatz-scout', 'Scout'),
  turn('disconfirmation-officer', 'Officer'),
  turn('epistemic-scribe', 'Scribe'),
  turn('device-auditor', 'Auditor'),
];

// Button children are icon + span arrays holding element instances
// (circular — no JSON). Walk for raw text instead.
function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(textOf).join('');
  if (node !== null && typeof node === 'object' && 'props' in node) {
    return textOf((node as { props: { children?: unknown } }).props.children);
  }
  return '';
}

function hasText(inst: TestRenderer.ReactTestInstance, text: string): boolean {
  return textOf(inst.props.children).includes(text);
}

describe('EpistemicTribunalRoom synthesize (U2-b)', () => {
  it('synthesize is disabled before consensus and posts the topic after', async () => {
    const seen: Array<{ url: string; body: string }> = [];
    let synthesized: unknown = null;
    stubFetch((url, init) => {
      if (url.includes('/api/chat/tribunal')) {
        return { ok: true, json: async () => ({ tribunal: { turns: TURNS } }) };
      }
      if (url.includes('/campaign/synthesize')) {
        seen.push({ url, body: String(init?.body) });
        return { ok: true, json: async () => ({ session_id: 's1', target_system: 'H2' }) };
      }
      throw new Error(`unexpected ${url}`);
    });

    const tree = await render(
      <EpistemicTribunalRoom
        sessionId="s1"
        targetSystem="H2"
        onCampaignSynthesized={(c) => { synthesized = c; }}
      />
    );
    const synthBtn = () =>
      tree.root.findAllByType('button').find((b) => hasText(b, 'Synthesize'))!;
    expect(synthBtn().props.disabled).toBe(true);

    await act(async () => {
      await tree.root.findAllByType('button').find((b) => hasText(b, 'Rerun'))!.props.onClick();
    });
    expect(synthBtn().props.disabled).toBe(false);

    await act(async () => {
      await synthBtn().props.onClick();
    });
    expect(seen).toHaveLength(1);
    expect(seen[0].url).toBe('http://localhost:8080/api/sessions/s1/campaign/synthesize');
    expect(JSON.parse(seen[0].body)).toEqual({ prompt: expect.stringContaining('H2') });
    expect(synthesized).toMatchObject({ target_system: 'H2' });
    expect(JSON.stringify(tree.toJSON())).toContain('ledger-sealed server-side');
  });

  it('refusal leaves the campaign untouched with a reason', async () => {
    stubFetch((url) => {
      if (url.includes('/api/chat/tribunal')) {
        return { ok: true, json: async () => ({ tribunal: { turns: TURNS } }) };
      }
      return { ok: false, status: 500, text: async () => 'llm unavailable' };
    });
    let called = false;
    const tree = await render(
      <EpistemicTribunalRoom sessionId="s1" onCampaignSynthesized={() => { called = true; }} />
    );
    await act(async () => {
      await tree.root.findAllByType('button').find((b) => hasText(b, 'Rerun'))!.props.onClick();
    });
    await act(async () => {
      await tree.root.findAllByType('button').find((b) => hasText(b, 'Synthesize'))!.props.onClick();
    });
    expect(called).toBe(false);
    expect(JSON.stringify(tree.toJSON())).toContain('campaign unchanged');
  });
});

describe('EpistemicTribunalRoom ledger-commit typed gate (Wave-2)', () => {
  async function reachConsensus(sessionId = 'sess-12345678') {
    const tree = await render(
      <EpistemicTribunalRoom sessionId={sessionId} targetSystem="H2" />
    );
    const commitBtn = () =>
      tree.root.findAllByType('button').find((b) => hasText(b, 'Commit Discovery'))!;
    // Quorum gating is exact: disabled before consensus.
    expect(commitBtn().props.disabled).toBe(true);
    await act(async () => {
      await tree.root.findAllByType('button').find((b) => hasText(b, 'Rerun'))!.props.onClick();
    });
    expect(commitBtn().props.disabled).toBe(false);
    return { tree, commitBtn };
  }

  function gateOf(tree: TestRenderer.ReactTestRenderer) {
    return tree.root.findByType(ConfirmDialog);
  }

  function gateConfirm(gate: TestRenderer.ReactTestInstance) {
    return gate
      .findAllByType('button')
      .find((b) => hasText(b, 'Commit to ledger') || hasText(b, 'Working'))!;
  }

  it('stages a typed short-id gate: wrong text disabled, exact text enables + commits', async () => {
    const seen: string[] = [];
    stubFetch((url, init) => {
      if (url.includes('/api/chat/tribunal')) {
        return { ok: true, json: async () => ({ tribunal: { turns: TURNS } }) };
      }
      if (url.includes('/api/ledger/append')) {
        seen.push(String(init?.body));
        return { ok: true, json: async () => ({ ok: true, record_hash: 'rh-1', event_id: 'e1' }) };
      }
      throw new Error(`unexpected ${url}`);
    });
    const { tree, commitBtn } = await reachConsensus('sess-12345678');
    // Single-click no more: the button stages the dialog, commits nothing.
    await act(async () => {
      commitBtn().props.onClick();
    });
    expect(seen).toHaveLength(0);
    expect(tree.root.findAllByType(ConfirmDialog)).toHaveLength(1);
    expect(gateOf(tree).props.requireText).toBe('sess-123');
    await act(async () => {
      gateOf(tree).findByType('input').props.onChange({ target: { value: 'sess-12' } });
    });
    expect(gateConfirm(gateOf(tree)).props.disabled).toBe(true);
    await act(async () => {
      gateOf(tree).findByType('input').props.onChange({ target: { value: 'sess-123' } });
    });
    expect(gateConfirm(gateOf(tree)).props.disabled).not.toBe(true);
    await act(async () => {
      gateOf(tree).props.onConfirm();
    });
    await vi.waitFor(() => expect(seen).toHaveLength(1));
    expect(JSON.parse(seen[0]).event_type).toBe('tribunal_commit');
    await vi.waitFor(() => expect(tree.root.findAllByType(ConfirmDialog)).toHaveLength(0));
    expect(JSON.stringify(tree.toJSON())).toContain('Committed to Merkle Ledger');
  });

  it('ledger refusal stays open in the dialog error slot (no silent close)', async () => {
    stubFetch((url) => {
      if (url.includes('/api/chat/tribunal')) {
        return { ok: true, json: async () => ({ tribunal: { turns: TURNS } }) };
      }
      if (url.includes('/api/ledger/append')) {
        return { ok: false, status: 403, text: async () => 'ledger refused: quorum stale' };
      }
      throw new Error(`unexpected ${url}`);
    });
    const { tree, commitBtn } = await reachConsensus();
    await act(async () => {
      commitBtn().props.onClick();
    });
    await act(async () => {
      gateOf(tree).findByType('input').props.onChange({ target: { value: 'sess-123' } });
    });
    await act(async () => {
      gateOf(tree).props.onConfirm();
    });
    await vi.waitFor(() =>
      expect(String(gateOf(tree).props.error)).toContain('quorum stale')
    );
    // Still open — nothing sealed.
    expect(tree.root.findAllByType(ConfirmDialog)).toHaveLength(1);
    expect(JSON.stringify(tree.toJSON())).not.toContain('Committed to Merkle Ledger');
  });

  it('backdrop/cancel dismisses without committing', async () => {
    stubFetch((url) => {
      if (url.includes('/api/chat/tribunal')) {
        return { ok: true, json: async () => ({ tribunal: { turns: TURNS } }) };
      }
      throw new Error(`unexpected ${url}`);
    });
    const { tree, commitBtn } = await reachConsensus();
    await act(async () => {
      commitBtn().props.onClick();
    });
    expect(tree.root.findAllByType(ConfirmDialog)).toHaveLength(1);
    await act(async () => {
      gateOf(tree).findByProps({ role: 'presentation' }).props.onClick();
    });
    expect(tree.root.findAllByType(ConfirmDialog)).toHaveLength(0);
    // Escape funnels to the same onCancel.
    await act(async () => {
      commitBtn().props.onClick();
    });
    await act(async () => {
      gateOf(tree).props.onCancel();
    });
    expect(tree.root.findAllByType(ConfirmDialog)).toHaveLength(0);
  });

  it('quorum display carries progressbar semantics with an accessible name', async () => {
    stubFetch((url) => {
      if (url.includes('/api/chat/tribunal')) {
        return { ok: true, json: async () => ({ tribunal: { turns: TURNS } }) };
      }
      throw new Error(`unexpected ${url}`);
    });
    const tree = await render(<EpistemicTribunalRoom sessionId="s1" targetSystem="H2" />);
    const bar = () => tree.root.findByProps({ role: 'progressbar' });
    expect(bar().props['aria-label']).toBe('Tribunal quorum consensus');
    expect(bar().props['aria-valuemin']).toBe(0);
    expect(bar().props['aria-valuemax']).toBe(100);
    await act(async () => {
      await tree.root.findAllByType('button').find((b) => hasText(b, 'Rerun'))!.props.onClick();
    });
    // Four corroborated turns → 100%.
    expect(bar().props['aria-valuenow']).toBe(100);
  });
});
