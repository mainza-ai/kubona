import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { EpistemicTribunalRoom } from './EpistemicTribunalRoom';

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
