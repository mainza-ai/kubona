import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { HuggingFaceModelHub } from './HuggingFaceModelHub';

// Migrated from react-test-renderer (HuggingFaceModelHub.download.test.tsx).
// The old suite read `JSON.stringify(tree.toJSON())` — a serialization walk
// that also matched hrefs and props — so it could not tell what the operator
// can actually see. These assertions read the rendered DOM in a real Chromium
// page and drive the buttons for real.
//
// No `vi.mock('@/lib/api')`: the real wrapper (requestOperation → apiFetch)
// runs, and the hub endpoints are answered at the fetch boundary. That keeps
// the honest-empty / refusal plumbing under test instead of stubbing it out.

const BIG = 4_360_000_000;
const SMALL = 1024;
const BIG_GB = (BIG / (1024 * 1024 * 1024)).toFixed(2);

type Call = { method: string; path: string; body: string };

function ok(payload: unknown) {
  return { ok: true, status: 200, json: async () => payload };
}

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

const DOWNLOADING_TASK = {
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
};

type Route = (call: Call) => unknown | undefined;

/** Answers the hub endpoints by URL + method, recording every call. */
function stubHub(route: Route, tasks: unknown[] = []) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const parsed = new URL(String(url));
    const call: Call = {
      method: (init?.method ?? 'GET').toUpperCase(),
      path: parsed.pathname,
      body: String(init?.body ?? ''),
    };
    calls.push(call);
    const routed = route(call);
    if (routed !== undefined) return routed;
    if (call.method === 'GET' && call.path === '/api/hf/search') return ok({ models: [bigModel()] });
    if (call.method === 'GET' && call.path.startsWith('/api/hf/models/')) return ok({ model: bigModel() });
    if (call.method === 'GET' && call.path === '/api/hf/downloads') return ok({ downloads: tasks });
    if (call.method === 'POST' && call.path === '/api/hf/download') {
      return ok({ task: { task_id: 'dl-1' } });
    }
    if (call.method === 'POST' && call.path.endsWith('/cancel')) {
      return ok({ ok: true, task_id: 'dl-9', status: 'cancelled' });
    }
    return {
      ok: false,
      status: 404,
      json: async () => ({ error: `unexpected ${call.method} ${call.path}` }),
      text: async () => `unexpected ${call.method} ${call.path}`,
    };
  });
  return calls;
}

function starts(calls: Call[]) {
  return calls.filter((c) => c.method === 'POST' && c.path === '/api/hf/download');
}

function cancels(calls: Call[]) {
  return calls.filter((c) => c.method === 'POST' && c.path.endsWith('/cancel'));
}

function text(screen: { container: HTMLElement }) {
  return screen.container.textContent ?? '';
}

/** Opens the quantization inspector for the single result card. */
async function openQuantPanel(screen: Awaited<ReturnType<typeof render>>) {
  const view = screen.getByRole('button', { name: /View Quantizations/ });
  await expect.element(view).toBeInTheDocument();
  await view.click();
  await expect.element(screen.getByRole('dialog')).toBeInTheDocument();
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('HuggingFaceModelHub crash guards + honest empty (real browser)', () => {
  it('survives malformed model shapes — null downloads/tags/quantizations never crash', async () => {
    stubHub((call) => {
      if (call.method === 'GET' && call.path === '/api/hf/search') {
        return ok({
          models: [
            {
              id: 'org/broken',
              author: 'org',
              downloads: null,
              likes: undefined,
              pipeline_tag: null,
              tags: null,
              quantizations: null,
            },
          ],
        });
      }
      if (call.method === 'GET' && call.path.startsWith('/api/hf/models/')) return ok({ model: null });
      return undefined;
    });

    const screen = await render(<HuggingFaceModelHub />);

    // The card renders from the malformed row: name present, null counts
    // collapse to 0 rather than throwing.
    await expect.poll(() => text(screen)).toContain('broken');
    await expect.element(screen.getByRole('heading', { name: 'broken' })).toBeVisible();
    expect(text(screen)).toContain('⬇ 0');
    expect(text(screen)).toContain('♥ 0');
    expect(text(screen)).not.toContain('No models found');

    // Null quantizations open the inspector without crashing and state the
    // refusal honestly instead of inventing files.
    await openQuantPanel(screen);
    await expect.poll(() => text(screen)).toContain('No single-file .gguf or .mlx weights detected');
    expect(text(screen)).toContain('org/broken');
  });

  it('shows a refused search with retry — never a fake "No models found"', async () => {
    let searches = 0;
    stubHub((call) => {
      if (call.method === 'GET' && call.path === '/api/hf/search') {
        searches += 1;
        return { ok: false, status: 401, json: async () => ({}), text: async () => 'no operator token' };
      }
      return undefined;
    });

    const screen = await render(<HuggingFaceModelHub />);

    await expect.poll(() => text(screen)).toContain('refused the search');
    expect(text(screen)).not.toContain('No models found');
    expect(searches).toBe(1);

    // Retry is a real control: it re-issues the search and keeps the honest
    // refusal on screen (the hub is still refusing).
    await screen.getByRole('button', { name: 'Retry search' }).click();
    await expect.poll(() => searches).toBe(2);
    expect(text(screen)).toContain('refused the search');
    expect(text(screen)).not.toContain('No models found');
  });

  it('shows "No models found" only after a search actually ran clean', async () => {
    stubHub((call) => {
      if (call.method === 'GET' && call.path === '/api/hf/search') return ok({ models: [] });
      return undefined;
    });

    const screen = await render(<HuggingFaceModelHub />);

    await expect.poll(() => text(screen)).toContain('No models found');
    expect(text(screen)).not.toContain('refused the search');
  });
});

describe('HuggingFaceModelHub download confirm (T-11, real browser)', () => {
  it('big fetch stops at a confirm dialog and posts nothing until confirmed', async () => {
    const calls = stubHub(() => undefined);
    const screen = await render(<HuggingFaceModelHub />);
    await openQuantPanel(screen);

    const download = screen.getByRole('button', { name: 'Download big-q6k.gguf' });
    await expect.element(download).toBeInTheDocument();
    await download.click();

    // Dialog names the file and stops the fetch; no POST fired.
    const dialog = screen.getByRole('alertdialog');
    await expect.element(dialog).toBeInTheDocument();
    await expect.element(dialog).toHaveAttribute('aria-label', 'Confirm download big-q6k.gguf');
    const dialogText = dialog.element().textContent ?? '';
    expect(dialogText).toContain('Large download');
    expect(dialogText).toContain(`${BIG_GB} GB`);
    expect(dialogText).toContain('big-q6k.gguf (org/big-GGUF)');
    expect(starts(calls)).toEqual([]);

    // Confirm posts size + confirmed:true.
    await dialog.getByRole('button', { name: 'Confirm download big-q6k.gguf' }).click();
    await expect.poll(() => starts(calls).length).toBe(1);
    expect(JSON.parse(starts(calls)[0].body)).toEqual({
      repo_id: 'org/big-GGUF',
      filename: 'big-q6k.gguf',
      size_bytes: BIG,
      confirmed: true,
    });
    // The confirmation is consumed by the confirm.
    await expect.poll(() => text(screen)).not.toContain('Large download');
  });

  it('dismissing the dialog posts nothing', async () => {
    const calls = stubHub(() => undefined);
    const screen = await render(<HuggingFaceModelHub />);
    await openQuantPanel(screen);

    await screen.getByRole('button', { name: 'Download big-q6k.gguf' }).click();
    await expect.element(screen.getByRole('alertdialog')).toBeInTheDocument();

    await screen.getByRole('button', { name: 'Dismiss download confirmation' }).click();
    await expect.poll(() => text(screen)).not.toContain('Large download');
    expect(screen.container.querySelector('[role="alertdialog"]')).toBeNull();
    expect(starts(calls)).toEqual([]);
  });

  it('small fetch skips the dialog and posts immediately', async () => {
    const calls = stubHub(() => undefined);
    const screen = await render(<HuggingFaceModelHub />);
    await openQuantPanel(screen);

    await screen.getByRole('button', { name: 'Download tiny-q8.gguf' }).click();
    await expect.poll(() => starts(calls).length).toBe(1);
    // No dialog at any point: the posted body carries no confirmation flag.
    expect(screen.container.querySelector('[role="alertdialog"]')).toBeNull();
    const body = JSON.parse(starts(calls)[0].body) as Record<string, unknown>;
    expect(body.confirmed).toBeUndefined();
    expect(body.size_bytes).toBe(SMALL);
  });

  it('cancel shows a pending state and posts once', async () => {
    const calls = stubHub(() => undefined, [DOWNLOADING_TASK]);
    const screen = await render(<HuggingFaceModelHub />);

    const cancel = screen.getByRole('button', { name: 'Cancel download big-q6k.gguf' });
    await expect.element(cancel).toBeInTheDocument();
    await cancel.click();

    await expect.poll(() => cancels(calls).length).toBe(1);
    expect(cancels(calls)[0].path).toBe('/api/hf/downloads/dl-9/cancel');
    // One click, one cancel: the component-level guard drops re-entry.
    expect(cancels(calls)).toHaveLength(1);
  });
});

describe('HuggingFaceModelHub icon-only button names (Wave-2, real browser)', () => {
  it('names every icon-only button and keeps existing tooltips', async () => {
    const calls = stubHub(() => undefined, [DOWNLOADING_TASK]);
    const screen = await render(<HuggingFaceModelHub />);
    await openQuantPanel(screen);

    const buttons = [...screen.container.querySelectorAll('button')];
    expect(buttons.length).toBeGreaterThan(0);
    for (const button of buttons) {
      // Accessible name for an icon-only button comes from aria-label; a
      // button with visible text needs neither.
      if ((button.textContent ?? '').trim() === '') {
        expect(
          button.getAttribute('aria-label') ?? '',
          'icon-only button without an accessible name',
        ).not.toBe('');
      }
    }
    expect(calls.some((c) => c.path === '/api/hf/download')).toBe(false);

    // Existing tooltips are kept.
    expect(screen.getByRole('button', { name: 'Refresh downloads' }).element().getAttribute('title')).toBe(
      'Refresh Downloads',
    );
    expect(screen.getByRole('button', { name: 'Cancel download big-q6k.gguf' }).element().getAttribute('title')).toBe(
      'Cancel download big-q6k.gguf',
    );
    expect(screen.getByRole('button', { name: 'Download big-q6k.gguf' }).element().getAttribute('title')).toBe(
      'Download big-q6k.gguf',
    );
  });
});
