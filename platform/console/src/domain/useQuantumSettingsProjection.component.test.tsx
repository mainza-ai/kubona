import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { resetSharedResourcesForTests } from './shared-resource';
import { useQuantumSettingsProjection } from './useQuantumSettingsProjection';

const SETTINGS = { default_backend: 'local-simulator' };

function Probe() {
  const { status, data } = useQuantumSettingsProjection();
  return <span data-testid="state">{`${status}:${data?.backends.length ?? 0}`}</span>;
}

function StatusProbe() {
  const { status } = useQuantumSettingsProjection();
  return <span data-testid="status">{status}</span>;
}

beforeEach(() => resetSharedResourcesForTests());
afterEach(() => vi.unstubAllGlobals());

describe('quantum settings projection in a real browser', () => {
  it('surfaces a refused settings read instead of an empty provider list', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) => (
      String(url).includes('/api/quantum/settings')
        ? {
            ok: false,
            status: 403,
            text: async () => 'quantum provider settings refused',
            json: async () => ({ message: 'quantum provider settings refused' }),
          }
        : { ok: true, status: 200, json: async () => ({ backends: [] }) }
    )));

    const screen = await render(<StatusProbe />);
    const status = screen.getByTestId('status');
    await expect.element(status).toHaveTextContent('refused');
    await screen.unmount();
  });

  it('shares one backends/settings fetch across co-mounted consumers', async () => {
    const fetchMock = vi.fn(async (url: string) => (
      String(url).includes('/api/quantum/settings')
        ? { ok: true, status: 200, json: async () => SETTINGS }
        : { ok: true, status: 200, json: async () => ({ backends: [] }) }
    ));
    vi.stubGlobal('fetch', fetchMock);

    const screen = await render(<><Probe /><Probe /></>);
    await expect.poll(() => document.body.textContent).toContain('ready:0');

    const calls = fetchMock.mock.calls.map((call) => String(call[0]));
    expect(calls.filter((url) => url.includes('/api/quantum/backends'))).toHaveLength(1);
    expect(calls.filter((url) => url.includes('/api/quantum/settings'))).toHaveLength(1);
    await screen.unmount();
  });
});
