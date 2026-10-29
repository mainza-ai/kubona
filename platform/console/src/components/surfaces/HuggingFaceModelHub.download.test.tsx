import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { HuggingFaceModelHub } from './HuggingFaceModelHub';

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(handler: (url: string, init?: RequestInit) => unknown) {
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => handler(url, init) as Response);
}

const BIG = 4_360_000_000;
const SMALL = 1024;

function bigModel() {
  return {
    id: 'org/big-GGUF',
    author: 'org',
    downloads: 10,
    likes: 1,
    pipeline_tag: 'text-generation',
    tags: ['gguf'],
    description: '',
    quantizations: [
      { filename: 'big-q6k.gguf', size_bytes: BIG, quant_type: 'Q6_K', download_url: '' },
      { filename: 'tiny-q8.gguf', size_bytes: SMALL, quant_type: 'Q8_0', download_url: '' },
    ],
  };
}

function stubHub(posted: Array<{ url: string; body: string }>, tasks: unknown[] = []) {
  stubFetch((url, init) => {
    if (url.includes('/api/hf/search')) {
      return { ok: true, status: 200, json: async () => ({ models: [bigModel()] }) };
    }
    if (url.includes('/api/hf/models/')) {
      return { ok: true, status: 200, json: async () => ({ model: bigModel() }) };
    }
    if (url.includes('/api/hf/downloads') && (!init || init.method !== 'POST')) {
      return { ok: true, status: 200, json: async () => ({ downloads: tasks }) };
    }
    if (url.endsWith('/api/hf/download')) {
      posted.push({ url, body: String(init?.body) });
      return { ok: true, status: 200, json: async () => ({ ok: true, task: { task_id: 'dl-1' } }) };
    }
    if (url.includes('/cancel')) {
      posted.push({ url, body: String(init?.body) });
      return { ok: true, status: 200, json: async () => ({ ok: true }) };
    }
    throw new Error(`unexpected ${url}`);
  });
}

async function render(): Promise<TestRenderer.ReactTestRenderer> {
  let tree: TestRenderer.ReactTestRenderer;
  await act(async () => {
    tree = TestRenderer.create(<HuggingFaceModelHub />);
  });
  return tree!;
}

async function openQuantPanel(tree: TestRenderer.ReactTestRenderer) {
  const span = tree.root
    .findAllByType('span')
    .find((s) => s.props.children === 'View Quantizations');
  const btn = span?.parent;
  if (!btn || typeof btn.props.onClick !== 'function') throw new Error('quant panel button not found');
  await act(async () => {
    await btn.props.onClick();
  });
}

describe('HuggingFaceModelHub crash guards + honest empty', () => {
  it('survives malformed model shapes — null downloads/tags/quantizations never crash', async () => {
    stubFetch((url) => {
      if (url.includes('/api/hf/search')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            models: [{ id: 'org/broken', author: 'org', downloads: null, likes: undefined, pipeline_tag: null, tags: null, quantizations: null }],
          }),
        };
      }
      if (url.includes('/api/hf/models/')) {
        return { ok: true, status: 200, json: async () => ({ model: null }) };
      }
      if (url.includes('/api/hf/downloads')) {
        return { ok: true, status: 200, json: async () => ({ downloads: [] }) };
      }
      throw new Error(`unexpected ${url}`);
    });
    const tree = await render();
    const html = JSON.stringify(tree.toJSON());
    expect(html).toContain('org/broken');
    expect(html).not.toContain('No models found');
  });

  it('shows a refused search with retry — never a fake "No models found"', async () => {
    stubFetch((url) => {
      if (url.includes('/api/hf/search')) {
        return { ok: false, status: 401, json: async () => ({}) };
      }
      if (url.includes('/api/hf/downloads')) {
        return { ok: true, status: 200, json: async () => ({ downloads: [] }) };
      }
      throw new Error(`unexpected ${url}`);
    });
    const tree = await render();
    const html = JSON.stringify(tree.toJSON());
    expect(html).toContain('refused the search');
    expect(html).toContain('Retry search');
    expect(html).not.toContain('No models found');
  });

  it('shows "No models found" only after a search actually ran clean', async () => {
    stubFetch((url) => {
      if (url.includes('/api/hf/search')) {
        return { ok: true, status: 200, json: async () => ({ models: [] }) };
      }
      if (url.includes('/api/hf/downloads')) {
        return { ok: true, status: 200, json: async () => ({ downloads: [] }) };
      }
      throw new Error(`unexpected ${url}`);
    });
    const tree = await render();
    const html = JSON.stringify(tree.toJSON());
    expect(html).toContain('No models found');
    expect(html).not.toContain('refused the search');
  });
});

describe('HuggingFaceModelHub download confirm (T-11)', () => {
  it('big fetch stops at a confirm dialog and posts nothing until confirmed', async () => {
    const posted: Array<{ url: string; body: string }> = [];
    stubHub(posted);
    const tree = await render();
    await openQuantPanel(tree);
    await act(async () => {
      await tree.root.findByProps({ 'aria-label': 'Download big-q6k.gguf' }).props.onClick();
    });
    // Dialog names the file and stops the fetch; no POST fired.
    const html = JSON.stringify(tree.toJSON());
    expect(html).toContain('Confirm download big-q6k.gguf');
    expect(html).toContain('Large download');
    expect(posted.filter((p) => p.url.endsWith('/api/hf/download'))).toEqual([]);
    // Confirm posts size + confirmed:true.
    const confirmBtn = tree.root
      .findAllByType('button')
      .find((b) => b.props['aria-label'] === 'Confirm download big-q6k.gguf');
    if (!confirmBtn || typeof confirmBtn.props.onClick !== 'function') {
      throw new Error('confirm button not found');
    }
    await act(async () => {
      await confirmBtn.props.onClick();
    });
    const starts = posted.filter((p) => p.url.endsWith('/api/hf/download'));
    expect(starts).toHaveLength(1);
    expect(JSON.parse(starts[0].body)).toEqual({
      repo_id: 'org/big-GGUF',
      filename: 'big-q6k.gguf',
      size_bytes: BIG,
      confirmed: true,
    });
  });

  it('dismissing the dialog posts nothing', async () => {
    const posted: Array<{ url: string; body: string }> = [];
    stubHub(posted);
    const tree = await render();
    await openQuantPanel(tree);
    await act(async () => {
      await tree.root.findByProps({ 'aria-label': 'Download big-q6k.gguf' }).props.onClick();
    });
    await act(async () => {
      await tree.root.findByProps({ 'aria-label': 'Dismiss download confirmation' }).props.onClick();
    });
    expect(posted.filter((p) => p.url.endsWith('/api/hf/download'))).toEqual([]);
    expect(JSON.stringify(tree.toJSON())).not.toContain('alertdialog');
  });

  it('small fetch skips the dialog and posts immediately', async () => {
    const posted: Array<{ url: string; body: string }> = [];
    stubHub(posted);
    const tree = await render();
    await openQuantPanel(tree);
    await act(async () => {
      await tree.root.findByProps({ 'aria-label': 'Download tiny-q8.gguf' }).props.onClick();
    });
    expect(JSON.stringify(tree.toJSON())).not.toContain('alertdialog');
    const starts = posted.filter((p) => p.url.endsWith('/api/hf/download'));
    expect(starts).toHaveLength(1);
    expect(JSON.parse(starts[0].body).confirmed).toBeUndefined();
  });

  it('cancel shows a pending state and posts once', async () => {
    const posted: Array<{ url: string; body: string }> = [];
    stubHub(posted, [
      {
        task_id: 'dl-9',
        repo_id: 'org/big-GGUF',
        filename: 'big-q6k.gguf',
        status: 'downloading',
        bytes_downloaded: 1,
        total_bytes: BIG,
        progress_percent: 1,
        speed_mb_s: 5,
        local_path: '/tmp/x',
        started_at: 1,
      },
    ]);
    const tree = await render();
    const btn = tree.root.findByProps({ 'aria-label': 'Cancel download big-q6k.gguf' });
    await act(async () => {
      await btn.props.onClick();
    });
    const cancels = posted.filter((p) => p.url.includes('/cancel'));
    expect(cancels).toHaveLength(1);
  });
});

describe('HuggingFaceModelHub icon-only button names (Wave-2)', () => {
  function buttonText(b: TestRenderer.ReactTestInstance): string {
    const walk = (n: unknown): string => {
      if (typeof n === 'string' || typeof n === 'number') return String(n);
      if (Array.isArray(n)) return n.map(walk).join('');
      if (n !== null && typeof n === 'object' && 'props' in n) {
        return walk((n as { props: { children?: unknown } }).props.children);
      }
      return '';
    };
    return walk((b.props as { children?: unknown }).children);
  }

  it('names every icon-only button and keeps existing tooltips', async () => {
    const posted: Array<{ url: string; body: string }> = [];
    stubHub(posted, [
      {
        task_id: 'dl-9',
        repo_id: 'org/big-GGUF',
        filename: 'big-q6k.gguf',
        status: 'downloading',
        bytes_downloaded: 1,
        total_bytes: BIG,
        progress_percent: 1,
        speed_mb_s: 5,
        local_path: '/tmp/x',
        started_at: 1,
      },
    ]);
    const tree = await render();
    await openQuantPanel(tree);
    const buttons = tree.root.findAllByType('button');
    expect(buttons.length).toBeGreaterThan(0);
    for (const b of buttons) {
      if (buttonText(b).trim() === '') {
        expect(
          String((b.props as { 'aria-label'?: unknown })['aria-label'] ?? ''),
          'icon-only button without an accessible name',
        ).not.toBe('');
      }
    }
    // Existing tooltips are kept.
    expect(tree.root.findByProps({ 'aria-label': 'Refresh downloads' }).props.title).toBe('Refresh Downloads');
    expect(tree.root.findByProps({ 'aria-label': 'Cancel download big-q6k.gguf' }).props.title).toBe('Cancel download big-q6k.gguf');
    expect(tree.root.findByProps({ 'aria-label': 'Download big-q6k.gguf' }).props.title).toBe('Download big-q6k.gguf');
  });
});
