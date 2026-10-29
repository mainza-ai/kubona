import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { LiteraturePanel } from './LiteraturePanel';

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(handler: (url: string) => unknown) {
  vi.stubGlobal('fetch', async (url: string) => handler(url) as Response);
}

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

describe('LiteraturePanel', () => {
  it('searches and renders records with real years and counts', async () => {
    stubFetch((url) => {
      if (url.includes('/api/literature')) {
        return {
          ok: true,
          json: async () => ({
            records: [
              { id: 'http://arxiv.org/abs/2101.00001v1', title: 'Test VQE paper', authors: 'Jane Researcher', abstract: 'Abstract here.', url: 'http://arxiv.org/abs/2101.00001v1', publication_year: 2021, citation_count: 0 },
            ],
            cached: false,
          }),
        };
      }
      throw new Error(`unexpected ${url}`);
    });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LiteraturePanel />);
    });
    const input = root!.root.findByType('input');
    await act(async () => {
      input.props.onChange({ target: { value: 'vqe' } });
    });
    const form = root!.root.findByType('form');
    await act(async () => {
      form.props.onSubmit({ preventDefault: () => {} });
    });
    const text = textOf(root!.root);
    expect(text).toContain('Test VQE paper');
    expect(text).toContain('2021');
    expect(text).toContain('live fetch');
  });

  it('labels a genuine empty result as no-matches, not unavailable', async () => {
    stubFetch((url) => {
      if (url.includes('/api/literature')) {
        return { ok: true, json: async () => ({ records: [], cached: false }) };
      }
      throw new Error(`unexpected ${url}`);
    });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LiteraturePanel initialQuery="vqe" />);
    });
    const form = root!.root.findByType('form');
    await act(async () => {
      form.props.onSubmit({ preventDefault: () => {} });
    });
    const text = textOf(root!.root);
    expect(text).toContain('no matches');
    expect(text).not.toContain('search unavailable');
  });

  it('labels transport failure as search-unavailable with retry, not no-matches', async () => {
    stubFetch(() => ({ ok: false, status: 503, json: async () => ({}) }));
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LiteraturePanel initialQuery="vqe" />);
    });
    const form = root!.root.findByType('form');
    await act(async () => {
      form.props.onSubmit({ preventDefault: () => {} });
    });
    const text = textOf(root!.root);
    expect(text).toContain('search unavailable');
    expect(text).toContain('Retry');
    expect(text).not.toContain('no matches');
  });

  it('labels non-empty cache hits as cache', async () => {
    stubFetch((url) => {
      if (url.includes('/api/literature')) {
        return {
          ok: true,
          json: async () => ({
            records: [
              { id: 'x', title: 'Cached paper', authors: 'A', abstract: '', url: 'x', publication_year: 2020, citation_count: 3 },
            ],
            cached: true,
          }),
        };
      }
      throw new Error(`unexpected ${url}`);
    });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LiteraturePanel initialQuery="vqe" />);
    });
    const form = root!.root.findByType('form');
    await act(async () => {
      form.props.onSubmit({ preventDefault: () => {} });
    });
    expect(textOf(root!.root)).toContain('served from local cache');
  });

  it('survives malformed record shapes without crashing', async () => {
    stubFetch((url) => {
      if (url.includes('/api/literature')) {
        return {
          ok: true,
          json: async () => ({
            records: [{ id: null, title: null, authors: null, abstract: null, url: null, publication_year: null, citation_count: null }],
            cached: false,
          }),
        };
      }
      throw new Error(`unexpected ${url}`);
    });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LiteraturePanel initialQuery="vqe" />);
    });
    const form = root!.root.findByType('form');
    await act(async () => {
      form.props.onSubmit({ preventDefault: () => {} });
    });
    const text = textOf(root!.root);
    expect(text).toContain('Untitled record');
    expect(text).toContain('live fetch');
  });
});
