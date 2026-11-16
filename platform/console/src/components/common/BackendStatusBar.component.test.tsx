import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { BackendStatusBar } from './BackendStatusBar';
import { resetSharedResourcesForTests } from '@/domain/shared-resource';

function stubFetch(handler: (url: string) => unknown) {
  vi.stubGlobal('fetch', async (url: string) => handler(url) as Response);
}

beforeEach(() => resetSharedResourcesForTests());
afterEach(() => vi.unstubAllGlobals());

async function renderStatus() {
  const screen = await render(<BackendStatusBar />);
  return screen.getByRole('status');
}

describe('BackendStatusBar in a real browser', () => {
  it('reports ready when health and ready both answer', async () => {
    stubFetch((url) => url.endsWith('/api/health')
      ? { ok: true, json: async () => ({ status: 'healthy', service: 'kubona-core-orchestrator', version: '0.2.0' }) }
      : { ok: true, json: async () => ({ ready: true }) });
    const status = await renderStatus();
    await expect.poll(() => status.element().getAttribute('aria-label')).toMatch(/ready/);
    expect(status.element().getAttribute('aria-label')).not.toContain('unreachable');
  });

  it('reports degraded with the backend reason on ready 503', async () => {
    stubFetch((url) => url.endsWith('/api/health')
      ? { ok: true, json: async () => ({ status: 'healthy', disk_level: 'blocked' }) }
      : { ok: false, status: 503, json: async () => ({ code: 'not_ready', message: 'not ready: disk blocked' }) });
    const status = await renderStatus();
    await expect.poll(() => status.element().getAttribute('aria-label')).toMatch(/degraded/);
    expect(status.element().getAttribute('aria-label')).toContain('disk blocked');
  });

  it('reports unreachable on transport failure', async () => {
    stubFetch(() => { throw new Error('connection refused'); });
    const status = await renderStatus();
    await expect.poll(() => status.element().getAttribute('aria-label')).toMatch(/unreachable/);
  });

  it('reports unreachable when health itself refuses', async () => {
    stubFetch(() => ({ ok: false, status: 502, json: async () => ({}) }));
    const status = await renderStatus();
    await expect.poll(() => status.element().getAttribute('aria-label')).toMatch(/unreachable/);
  });
});
