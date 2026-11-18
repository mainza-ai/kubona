import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { PublicationPanel } from './PublicationPanel';

type Call = { method: string; path: string; body: Record<string, unknown> | undefined };

const SUMMARY = {
  id: 'ms_1',
  title: 'Measured H2 results',
  state: 'draft',
  content_hash: 'abcdef1234567890',
  created_at: '2026-11-18T00:00:00Z',
  updated_at: '2026-11-18T00:00:00Z',
};

function detail(over: Record<string, unknown> = {}) {
  return {
    manuscript: {
      ...SUMMARY,
      session_id: 's1',
      abstract: 'Measured results only.',
      build_hash: '',
      review: { recommendation: 'REVISE', mean_score: 6.9, hygiene: 4 },
      source_date_epoch: 1700000000,
      sections: [{ slug: 'results', title: 'Results', latex: 'x', position: 0 }],
      citations: [],
      artifacts: [],
      signoff: null,
      ...over,
    },
    reviews: [{ id: 'r1', reviewer: 'automated_peer_review', recommendation: 'REVISE', acceptable: false, mean_score: 6.9, hygiene: 4, content_hash: 'abcdef1234567890', created_at: '2026-11-18T00:00:00Z' }],
  };
}

function json(body: unknown, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

function installFetch(overrides: {
  list?: unknown[];
  detail?: unknown;
  create?: () => unknown;
  build?: () => unknown;
  signoff?: () => unknown;
  deposit?: () => unknown;
} = {}) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const parsed = new URL(String(url));
    const path = parsed.pathname;
    const method = (init?.method ?? 'GET').toUpperCase();
    const body = init?.body === undefined ? undefined : JSON.parse(String(init.body)) as Record<string, unknown>;
    calls.push({ method, path, body });

    if (method === 'GET' && /^\/api\/sessions\/[^/]+\/manuscripts$/.test(path)) {
      return json({ manuscripts: overrides.list ?? [] });
    }
    if (method === 'GET' && /^\/api\/sessions\/[^/]+\/manuscripts\/[^/]+$/.test(path)) {
      return json(overrides.detail ?? detail());
    }
    if (method === 'POST' && /^\/api\/sessions\/[^/]+\/manuscripts$/.test(path)) {
      return overrides.create ? overrides.create() : json(detail(), 201);
    }
    if (method === 'POST' && path.endsWith('/build')) {
      return overrides.build ? overrides.build() : json(detail({ state: 'built', build_hash: 'build-1', artifacts: [{ name: 'main.pdf', sha256: 'f'.repeat(64), size: 10, created_at: '2026-11-18T00:00:00Z' }] }));
    }
    if (method === 'POST' && path.endsWith('/reviews')) {
      return json(detail({ state: 'reviewed' }));
    }
    if (method === 'POST' && path.endsWith('/signoff')) {
      return overrides.signoff ? overrides.signoff() : json(detail({ state: 'signed', signoff: { signer_pubkey_hex: 'aa', signature_hex: 'bb', content_hash: 'abcdef1234567890', expiry: '2030-01-01T00:00:00Z', signed_at: '2026-11-18T00:00:00Z' } }));
    }
    if (method === 'POST' && path.endsWith('/deposit')) {
      return overrides.deposit ? overrides.deposit() : json({ ok: true, manifest: { state: 'staged-needs-human-upload' } });
    }
    throw new Error(`unexpected fetch ${method} ${path}`);
  });
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('PublicationPanel', () => {
  it('shows an explicit empty state and creates a server-side draft', async () => {
    const calls = installFetch();
    const screen = await render(<PublicationPanel sessionId="s1" />);
    await vi.waitFor(() => expect(screen.container.textContent).toContain('No manuscript yet'));

    const create = Array.from(screen.container.querySelectorAll('button')).find((b) => b.textContent?.includes('New draft'));
    expect(create).toBeTruthy();
    create!.click();

    await vi.waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.path === '/api/sessions/s1/manuscripts')).toBe(true));
    await vi.waitFor(() => expect(screen.container.textContent).toContain('Draft created from the stored campaign.'));
  });

  it('builds a PDF and exposes the artifact download with its recorded hash', async () => {
    installFetch({ list: [SUMMARY], detail: detail() });
    const screen = await render(<PublicationPanel sessionId="s1" />);
    await vi.waitFor(() => expect(screen.container.textContent).toContain('Measured H2 results'));

    const build = Array.from(screen.container.querySelectorAll('button')).find((b) => b.textContent?.includes('Build PDF'));
    build!.click();

    await vi.waitFor(() => expect(screen.container.textContent).toContain('Built the PDF from the stored source.'));
    await vi.waitFor(() => expect(screen.container.textContent).toContain('main.pdf'));
    const link = screen.container.querySelector('a[download="main.pdf"]') as HTMLAnchorElement | null;
    expect(link?.getAttribute('href')).toContain('/api/sessions/s1/manuscripts/ms_1/artifacts/main.pdf');
  });

  it('surfaces the operator-only signoff refusal verbatim', async () => {
    installFetch({
      list: [SUMMARY],
      detail: detail(),
      signoff: () => json({ code: 'forbidden', message: 'manuscript signoff requires the operator bearer' }, 403),
    });
    const screen = await render(<PublicationPanel sessionId="s1" />);
    await vi.waitFor(() => expect(screen.container.textContent).toContain('Measured H2 results'));

    const sign = Array.from(screen.container.querySelectorAll('button')).find((b) => b.textContent?.includes('Sign off'));
    sign!.click();

    await vi.waitFor(() => expect(screen.container.textContent).toContain('manuscript signoff requires the operator bearer'));
  });

  it('disables deposit staging until the manuscript is signed and accepted', async () => {
    installFetch({ list: [SUMMARY], detail: detail() });
    const screen = await render(<PublicationPanel sessionId="s1" />);
    await vi.waitFor(() => expect(screen.container.textContent).toContain('Measured H2 results'));

    const deposit = Array.from(screen.container.querySelectorAll('button')).find((b) => b.textContent?.includes('Stage deposit')) as HTMLButtonElement | undefined;
    expect(deposit?.disabled).toBe(true);
  });
});
