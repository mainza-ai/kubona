import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import { SessionPortability } from './SessionPortability';

// U2-a / T-13 portability surfaces in a real Chromium page: the export and
// import paths are exercised through their real controls (button clicks and a
// real file input), and fetch is stubbed at the network boundary so the
// component's own URL/method/body contract stays under test.
afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(handler: (url: string, init?: RequestInit) => unknown) {
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => handler(String(url), init) as Response);
}

function manifestFile(text: string, name = 'manifest.json'): File {
  return new File([text], name, { type: 'application/json' });
}

function fileInput(screen: { container: HTMLElement }): HTMLInputElement {
  const input = screen.container.querySelector('input[type="file"]');
  expect(input).not.toBeNull();
  return input as HTMLInputElement;
}

describe('SessionPortability (U2-a) in a real browser', () => {
  it('export posts the session id and ledger JSON posts the format', async () => {
    const seen: Array<{ url: string; body: string }> = [];
    stubFetch((url, init) => {
      seen.push({ url, body: String(init?.body) });
      if (url.endsWith('/api/sessions/export')) {
        return { ok: true, status: 200, json: async () => ({ version: 1 }) };
      }
      return { ok: true, status: 200, json: async () => ({ ok: true, events: [] }) };
    });
    const screen = await render(<SessionPortability sessionId="s1" onSessionImported={() => {}} />);

    const exportSession = screen.getByRole('button', { name: 'Export session manifest' });
    await exportSession.click();
    await expect.poll(() => seen.length).toBe(1);
    // The button is disabled while the export is in flight; wait for the
    // component to settle before clicking the ledger export.
    await expect.poll(() => (exportSession.element() as HTMLButtonElement).disabled).toBe(false);

    await screen.getByRole('button', { name: 'Export ledger as JSON' }).click();
    await expect.poll(() => seen.length).toBe(2);

    expect(seen[0]).toEqual({
      url: 'http://localhost:8080/api/sessions/export',
      body: JSON.stringify({ session_id: 's1' }),
    });
    expect(seen[1].url).toBe('http://localhost:8080/api/ledger/export');
    expect(JSON.parse(seen[1].body)).toEqual({ session_id: 's1', format: 'json' });
  });

  it('import parses client-side and hands the new session to the parent', async () => {
    const seen: Array<{ url: string; body: string }> = [];
    let imported: [string, boolean] | null = null;
    const payload = JSON.stringify({ session_id: 's2', manifest_version: 1 });
    stubFetch((url, init) => {
      seen.push({ url, body: String(init?.body) });
      return { ok: true, status: 200, text: async () => payload, json: async () => JSON.parse(payload) };
    });
    const screen = await render(
      <SessionPortability
        sessionId="s1"
        onSessionImported={(id, replayed) => {
          imported = [id, replayed];
        }}
      />,
    );

    await userEvent.upload(fileInput(screen), manifestFile('{"version":1}'));

    await expect.poll(() => seen.length).toBe(1);
    expect(seen[0].url).toBe('http://localhost:8080/api/sessions/import');
    expect(JSON.parse(seen[0].body)).toEqual({ manifest: { version: 1 } });
    await expect.poll(() => imported).toEqual(['s2', false]);
  });

  it('invalid JSON never reaches the network', async () => {
    let calls = 0;
    stubFetch(() => {
      calls++;
      return { ok: true, status: 200, json: async () => ({}) };
    });
    const screen = await render(<SessionPortability sessionId="s1" onSessionImported={() => {}} />);

    await userEvent.upload(fileInput(screen), manifestFile('not json{{'));

    const alert = screen.getByRole('alert');
    await expect.element(alert).toBeVisible();
    expect(alert.element().textContent).toContain('not valid JSON');
    expect(calls).toBe(0);
  });

  it('backend refusal renders verbatim (unknown session)', async () => {
    stubFetch(() => ({
      ok: false,
      status: 404,
      text: async () => 'unknown session "ghost": nothing to export',
    }));
    const screen = await render(<SessionPortability sessionId="ghost" onSessionImported={() => {}} />);

    await screen.getByRole('button', { name: 'Export session manifest' }).click();

    const alert = screen.getByRole('alert');
    await expect.element(alert).toBeVisible();
    expect(alert.element().textContent).toContain('unknown session "ghost": nothing to export');
  });
});

describe('SessionPortability import receipt (T-13) in a real browser', () => {
  async function importManifest(responseBody: unknown): Promise<HTMLElement> {
    stubFetch(() => ({
      ok: true,
      status: 200,
      text: async () => JSON.stringify(responseBody),
      json: async () => responseBody,
    }));
    const screen = await render(<SessionPortability sessionId="s1" onSessionImported={() => {}} />);
    await userEvent.upload(fileInput(screen), manifestFile('{"version":1}'));
    const alert = screen.getByRole('alert');
    await expect.element(alert).toBeVisible();
    return alert.element() as HTMLElement;
  }

  it('fresh import renders a success receipt naming the session', async () => {
    const alert = await importManifest({ session_id: 's2-fresh', replayed: false });
    const text = alert.textContent ?? '';
    expect(text).toContain('imported session s2-fresh');
    expect(text).toContain('switched to it');
    expect(alert.className).toContain('text-success-ink');
    expect(alert.getAttribute('role')).toBe('alert');
  });

  it('idempotent replay says so instead of claiming a fresh import', async () => {
    const alert = await importManifest({ session_id: 's2-old', replayed: true });
    const text = alert.textContent ?? '';
    expect(text).toContain('already present');
    expect(text).toContain('replayed without duplicating');
    expect(text).not.toContain('imported session s2-old');
  });
});
