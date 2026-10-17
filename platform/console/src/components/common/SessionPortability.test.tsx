import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { SessionPortability } from './SessionPortability';

afterEach(() => {
  vi.unstubAllGlobals();
});

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

describe('SessionPortability (U2-a)', () => {
  it('export posts the session id and ledger JSON posts the format', async () => {
    const seen: Array<{ url: string; body: string }> = [];
    stubFetch((url, init) => {
      seen.push({ url, body: String(init?.body) });
      if (url.endsWith('/api/sessions/export')) {
        return { ok: true, status: 200, json: async () => ({ version: 1 }) };
      }
      return { ok: true, status: 200, json: async () => ({ ok: true, events: [] }) };
    });
    const tree = await render(<SessionPortability sessionId="s1" onSessionImported={() => {}} />);
    await act(async () => {
      await tree.root.findByProps({ 'aria-label': 'Export session manifest' }).props.onClick();
    });
    await act(async () => {
      await tree.root.findByProps({ 'aria-label': 'Export ledger as JSON' }).props.onClick();
    });
    expect(seen[0]).toEqual({
      url: 'http://localhost:8080/api/sessions/export',
      body: JSON.stringify({ session_id: 's1' }),
    });
    expect(JSON.parse(seen[1].body)).toEqual({ session_id: 's1', format: 'json' });
  });

  it('import parses client-side and hands the new session to the parent', async () => {
    const seen: string[] = [];
    let imported: [string, boolean] | null = null;
    const payload = JSON.stringify({ session_id: 's2', manifest_version: 1 });
    stubFetch((url) => {
      seen.push(url);
      return { ok: true, status: 200, text: async () => payload, json: async () => JSON.parse(payload) };
    });
    const tree = await render(
      <SessionPortability sessionId="s1" onSessionImported={(id, replayed) => { imported = [id, replayed]; }} />
    );
    const input = tree.root.findByProps({ 'aria-label': 'Session manifest file' });
    await act(async () => {
      await input.props.onChange({
        target: { files: [{ text: async () => '{"version":1}' }], value: '' },
      });
    });
    expect(seen).toEqual(['http://localhost:8080/api/sessions/import']);
    expect(imported).toEqual(['s2', false]);
  });

  it('invalid JSON never reaches the network', async () => {
    let calls = 0;
    stubFetch(() => {
      calls++;
      return { ok: true, status: 200, json: async () => ({}) };
    });
    const tree = await render(<SessionPortability sessionId="s1" onSessionImported={() => {}} />);
    const input = tree.root.findByProps({ 'aria-label': 'Session manifest file' });
    await act(async () => {
      await input.props.onChange({
        target: { files: [{ text: async () => 'not json{{' }], value: '' },
      });
    });
    expect(calls).toBe(0);
    expect(JSON.stringify(tree.toJSON())).toContain('not valid JSON');
  });

  it('backend refusal renders verbatim (unknown session)', async () => {
    stubFetch(() => ({
      ok: false,
      status: 404,
      text: async () => 'unknown session "ghost": nothing to export',
    }));
    const tree = await render(<SessionPortability sessionId="ghost" onSessionImported={() => {}} />);
    await act(async () => {
      await tree.root.findByProps({ 'aria-label': 'Export session manifest' }).props.onClick();
    });
    expect(JSON.stringify(tree.toJSON())).toContain('nothing to export');
  });
});
