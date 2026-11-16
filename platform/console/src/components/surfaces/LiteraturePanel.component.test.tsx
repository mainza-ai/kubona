import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { LiteraturePanel } from './LiteraturePanel';

// Migrated from react-test-renderer (LiteraturePanel.test.tsx). The old suite
// invoked input.props.onChange and form.props.onSubmit directly; here the
// operator types into the real input and clicks the real submit button, so
// the form path (submit → preventDefault → run) is exercised as shipped.
afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(handler: (url: string, init?: RequestInit) => unknown) {
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => handler(String(url), init));
}

function literatureOk(body: unknown) {
  return { ok: true, status: 200, json: async () => body };
}

describe('LiteraturePanel in a real browser', () => {
  it('searches and renders records with real years and counts', async () => {
    stubFetch((url) => {
      if (url.includes('/api/literature')) {
        return literatureOk({
          records: [
            { id: 'http://arxiv.org/abs/2101.00001v1', title: 'Test VQE paper', authors: 'Jane Researcher', abstract: 'Abstract here.', url: 'http://arxiv.org/abs/2101.00001v1', publication_year: 2021, citation_count: 0 },
          ],
          cached: false,
        });
      }
      throw new Error(`unexpected ${url}`);
    });
    const screen = await render(<LiteraturePanel />);
    await screen.getByLabelText('Search literature').fill('vqe');
    await screen.getByRole('button', { name: 'Search' }).click();
    await expect.poll(() => screen.container.textContent ?? '').toContain('Test VQE paper');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('2021');
    expect(text).toContain('live fetch');
  });

  it('labels a genuine empty result as no-matches, not unavailable', async () => {
    stubFetch((url) => {
      if (url.includes('/api/literature')) return literatureOk({ records: [], cached: false });
      throw new Error(`unexpected ${url}`);
    });
    const screen = await render(<LiteraturePanel initialQuery="vqe" />);
    await screen.getByRole('button', { name: 'Search' }).click();
    await expect.poll(() => screen.container.textContent ?? '').toContain('no matches');
    const text = screen.container.textContent ?? '';
    expect(text).not.toContain('search unavailable');
  });

  it('labels transport failure as search-unavailable with retry, not no-matches', async () => {
    stubFetch(() => ({ ok: false, status: 503, json: async () => ({}) }));
    const screen = await render(<LiteraturePanel initialQuery="vqe" />);
    await screen.getByRole('button', { name: 'Search' }).click();
    await expect.poll(() => screen.container.textContent ?? '').toContain('search unavailable');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('Retry');
    expect(text).not.toContain('no matches');
  });

  it('labels non-empty cache hits as cache', async () => {
    stubFetch((url) => {
      if (url.includes('/api/literature')) {
        return literatureOk({
          records: [
            { id: 'x', title: 'Cached paper', authors: 'A', abstract: '', url: 'x', publication_year: 2020, citation_count: 3 },
          ],
          cached: true,
        });
      }
      throw new Error(`unexpected ${url}`);
    });
    const screen = await render(<LiteraturePanel initialQuery="vqe" />);
    await screen.getByRole('button', { name: 'Search' }).click();
    await expect.poll(() => screen.container.textContent ?? '').toContain('served from local cache');
  });

  it('survives malformed record shapes without crashing', async () => {
    stubFetch((url) => {
      if (url.includes('/api/literature')) {
        return literatureOk({
          records: [{ id: null, title: null, authors: null, abstract: null, url: null, publication_year: null, citation_count: null }],
          cached: false,
        });
      }
      throw new Error(`unexpected ${url}`);
    });
    const screen = await render(<LiteraturePanel initialQuery="vqe" />);
    await screen.getByRole('button', { name: 'Search' }).click();
    await expect.poll(() => screen.container.textContent ?? '').toContain('Untitled record');
    expect(screen.container.textContent ?? '').toContain('live fetch');
  });
});
