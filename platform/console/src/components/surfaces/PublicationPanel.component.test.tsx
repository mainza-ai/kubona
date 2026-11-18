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
    annotations: [],
    open_annotations: 0,
  };
}

function setInputValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event('input', { bubbles: true }));
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
  let annotations: Array<Record<string, unknown>> = [];
  let citations: Array<Record<string, unknown>> = [];
  const detailWithNotes = () => {
    const base = (overrides.detail ?? detail()) as { manuscript: Record<string, unknown>; reviews: unknown[] };
    return {
      ...base,
      annotations,
      open_annotations: annotations.filter((a) => a.status === 'open').length,
      manuscript: { ...base.manuscript, citations },
    };
  };
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
      return json(detailWithNotes());
    }
    if (method === 'GET' && /\/annotations$/.test(path)) {
      return json({ annotations });
    }
    if (method === 'POST' && /\/annotations$/.test(path)) {
      const annotation = {
        id: `ann_${annotations.length + 1}`,
        manuscript_id: 'ms_1',
        author: 'operator',
        anchor: body?.anchor ?? 'general',
        body: body?.body ?? '',
        status: 'open',
        content_hash: 'abcdef1234567890',
        created_at: '2026-11-18T00:00:00Z',
        updated_at: '2026-11-18T00:00:00Z',
      };
      annotations = [annotation, ...annotations];
      return json({ annotation }, 201);
    }
    if (method === 'PATCH' && /\/annotations\/[^/]+$/.test(path)) {
      const id = path.split('/').pop();
      annotations = annotations.map((a) => (a.id === id ? { ...a, status: body?.status } : a));
      return json(detailWithNotes());
    }
    if (method === 'GET' && path === '/api/literature') {
      return json({ cached: false, records: [
        { id: 'W123', title: 'Prior H2 study', authors: 'Doe, J.', abstract: '', url: '', publication_year: 2024, citation_count: 3 },
      ] });
    }
    if (method === 'PUT' && /\/citations$/.test(path)) {
      citations = (body?.citations as Array<Record<string, unknown>>) ?? [];
      return json(detailWithNotes());
    }
    if (method === 'GET' && path.includes('/artifacts/')) {
      return { ok: true, status: 200, json: async () => ({}), blob: async () => new Blob(['%PDF-1.4']) };
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
    // Artifact bytes go through the authenticated blob download, not a bare
    // anchor (which would drop the bearer header in a bearer-only console).
    const download = Array.from(screen.container.querySelectorAll('button')).find((b) => b.textContent?.includes('download'));
    expect(download).toBeTruthy();
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
    expect(screen.container.textContent).toContain('requires a comparator-linked kernel proof');
  });
});


describe('PublicationPanel review notes', () => {
  it('persists a note and blocks signoff while it is open', async () => {
    const calls = installFetch({ list: [SUMMARY], detail: detail() });
    const screen = await render(<PublicationPanel sessionId="s1" />);
    await vi.waitFor(() => expect(screen.container.textContent).toContain('Measured H2 results'));

    const input = screen.container.querySelector('input[aria-label="Review note"]') as HTMLInputElement;
    setInputValue(input, 'Explain the 15 mHa gap.');
    const add = Array.from(screen.container.querySelectorAll('button')).find((b) => b.textContent?.includes('Add note'));
    add!.click();

    await vi.waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.path.endsWith('/annotations'))).toBe(true));
    await vi.waitFor(() => expect(screen.container.textContent).toContain('1 open'));
    const sign = Array.from(screen.container.querySelectorAll('button')).find((b) => b.textContent?.includes('Sign off')) as HTMLButtonElement | undefined;
    expect(sign?.disabled).toBe(true);
  });

  it('resolves a note and unblocks signoff', async () => {
    const calls = installFetch({ list: [SUMMARY], detail: detail() });
    const screen = await render(<PublicationPanel sessionId="s1" />);
    await vi.waitFor(() => expect(screen.container.textContent).toContain('Measured H2 results'));

    const input = screen.container.querySelector('input[aria-label="Review note"]') as HTMLInputElement;
    setInputValue(input, 'Check the anchor.');
    Array.from(screen.container.querySelectorAll('button')).find((b) => b.textContent?.includes('Add note'))!.click();
    await vi.waitFor(() => expect(screen.container.textContent).toContain('1 open'));

    Array.from(screen.container.querySelectorAll('button')).find((b) => b.textContent?.includes('Mark addressed'))!.click();
    await vi.waitFor(() => expect(calls.some((c) => c.method === 'PATCH' && c.path.includes('/annotations/'))).toBe(true));
    await vi.waitFor(() => expect(screen.container.textContent).toContain('0 open'));
    const sign = Array.from(screen.container.querySelectorAll('button')).find((b) => b.textContent?.includes('Sign off')) as HTMLButtonElement | undefined;
    expect(sign?.disabled).toBe(false);
  });
});


describe('PublicationPanel artifacts and review', () => {
  it('downloads an artifact through the authenticated transport', async () => {
    const calls = installFetch({
      list: [SUMMARY],
      detail: detail({
        artifacts: [{ name: 'sections/results.tex', sha256: 'f'.repeat(64), size: 10, created_at: '2026-11-18T00:00:00Z' }],
      }),
    });
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:fake');
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});

    const screen = await render(<PublicationPanel sessionId="s1" />);
    await vi.waitFor(() => expect(screen.container.textContent).toContain('sections/results.tex'));
    const download = Array.from(screen.container.querySelectorAll('button')).find((b) => b.textContent?.includes('download'));
    download!.click();

    await vi.waitFor(() => expect(calls.some((c) => c.method === 'GET' && c.path.endsWith('/artifacts/sections/results.tex'))).toBe(true));
  });

  it('shows the review dimensions, including unmeasured values', async () => {
    installFetch({ list: [SUMMARY], detail: detail() });
    const screen = await render(<PublicationPanel sessionId="s1" />);
    await vi.waitFor(() => expect(screen.container.textContent).toContain('Measured H2 results'));
    const summary = Array.from(screen.container.querySelectorAll('summary')).find((s) => s.textContent?.includes('Review dimensions'));
    summary!.click();
    await vi.waitFor(() => expect(screen.container.textContent).toContain('6.90'));
    expect(screen.container.textContent).toContain('unmeasured');
  });
});


describe('PublicationPanel references', () => {
  it('searches literature and adds a reference to a draft', async () => {
    const calls = installFetch({ list: [SUMMARY], detail: detail() });
    const screen = await render(<PublicationPanel sessionId="s1" />);
    await vi.waitFor(() => expect(screen.container.textContent).toContain('Measured H2 results'));

    const search = screen.container.querySelector('input[aria-label="Literature search"]') as HTMLInputElement;
    setInputValue(search, 'H2 ground state');
    Array.from(screen.container.querySelectorAll('button')).find((b) => b.textContent?.includes('Search'))!.click();
    await vi.waitFor(() => expect(screen.container.textContent).toContain('Prior H2 study'));

    Array.from(screen.container.querySelectorAll('button')).find((b) => b.textContent?.trim() === 'Add')!.click();
    await vi.waitFor(() => expect(calls.some((c) => c.method === 'PUT' && c.path.endsWith('/citations'))).toBe(true));
    await vi.waitFor(() => expect(screen.container.textContent).toContain('References updated'));
  });
});
