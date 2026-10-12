import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { LiteraturePanel } from './LiteraturePanel';
import * as api from '@/lib/api';

vi.mock('@/lib/api', () => ({
  searchLiterature: vi.fn(),
}));

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

describe('LiteraturePanel', () => {
  it('searches and renders records with real years and counts', async () => {
    vi.mocked(api.searchLiterature).mockResolvedValue({
      records: [
        { id: 'http://arxiv.org/abs/2101.00001v1', title: 'Test VQE paper', authors: 'Jane Researcher', abstract: 'Abstract here.', url: 'http://arxiv.org/abs/2101.00001v1', publication_year: 2021, citation_count: 0 },
      ],
      cached: false,
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
    expect(vi.mocked(api.searchLiterature)).toHaveBeenCalledWith('vqe');
  });

  it('labels cached results honestly', async () => {
    vi.mocked(api.searchLiterature).mockResolvedValue({ records: [], cached: true });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<LiteraturePanel initialQuery="vqe" />);
    });
    const form = root!.root.findByType('form');
    await act(async () => {
      form.props.onSubmit({ preventDefault: () => {} });
    });
    expect(textOf(root!.root)).toContain('no records returned');
  });

  it('labels non-empty cache hits as cache', async () => {
    vi.mocked(api.searchLiterature).mockResolvedValue({
      records: [
        { id: 'x', title: 'Cached paper', authors: 'A', abstract: '', url: 'x', publication_year: 2020, citation_count: 3 },
      ],
      cached: true,
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
});
