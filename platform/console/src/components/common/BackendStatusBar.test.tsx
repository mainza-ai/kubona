import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { BackendStatusBar } from './BackendStatusBar';

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(handler: (url: string) => unknown) {
  vi.stubGlobal(
    'fetch',
    async (url: string) => handler(url) as Response
  );
}

async function render(): Promise<TestRenderer.ReactTestRenderer> {
  let tree: TestRenderer.ReactTestRenderer;
  await act(async () => {
    tree = TestRenderer.create(<BackendStatusBar />);
  });
  return tree!;
}

describe('BackendStatusBar (U1-a)', () => {
  it('reports healthy when health 200 and ready 200', async () => {
    stubFetch((url) =>
      url.endsWith('/api/health')
        ? { ok: true, json: async () => ({ status: 'healthy', service: 'kubona-core-orchestrator', version: '0.2.0' }) }
        : { ok: true, json: async () => ({ ready: true }) }
    );
    const text = JSON.stringify((await render()).toJSON());
    expect(text).toContain('ready');
    expect(text).not.toContain('unreachable');
    expect(text).not.toContain('degraded');
  });

  it('reports degraded with the backend reason when ready 503s', async () => {
    stubFetch((url) =>
      url.endsWith('/api/health')
        ? { ok: true, json: async () => ({ status: 'healthy', disk_level: 'blocked' }) }
        : { ok: false, status: 503, json: async () => ({ code: 'not_ready', message: 'not ready: disk blocked' }) }
    );
    const text = JSON.stringify((await render()).toJSON());
    expect(text).toContain('degraded');
    expect(text).toContain('disk blocked');
  });

  it('reports unreachable when transport fails (never renders empty as healthy)', async () => {
    stubFetch(() => {
      throw new Error('connection refused');
    });
    const text = JSON.stringify((await render()).toJSON());
    expect(text).toContain('unreachable');
  });

  it('reports unreachable when health itself refuses', async () => {
    stubFetch(() => ({ ok: false, status: 502, json: async () => ({}) }));
    const text = JSON.stringify((await render()).toJSON());
    expect(text).toContain('unreachable');
  });
});
