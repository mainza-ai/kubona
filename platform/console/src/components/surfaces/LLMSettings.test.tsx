import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { LLMSettings } from './LLMSettings';

afterEach(() => {
  vi.unstubAllGlobals();
});

function settings() {
  return {
    active_provider: 'nvidia',
    active_model: 'nvidia/fleet-model',
    air_gapped: false,
    temperature: 0.2,
    max_tokens: 0,
    providers: {
      nvidia: { type: 'nvidia', name: 'NVIDIA NIM', base_url: 'https://integrate.api.nvidia.com/v1', default_model: 'nvidia/fleet-model', enabled: true, is_local: false, timeout_sec: 120 },
      omlx: { type: 'omlx', name: 'oMLX', base_url: 'http://127.0.0.1:8787/v1', default_model: '', enabled: true, is_local: true, timeout_sec: 30 },
    },
    role_assignments: {
      copilot: { role: 'copilot', provider: 'nvidia', model: 'nvidia/fleet-model' },
    },
  };
}

function stubFetch(handler: (url: string, init?: RequestInit) => unknown) {
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => handler(url, init) as Response);
}

async function render(): Promise<TestRenderer.ReactTestRenderer> {
  let tree: TestRenderer.ReactTestRenderer;
  await act(async () => {
    tree = TestRenderer.create(<LLMSettings />);
  });
  return tree!;
}

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(textOf).join('');
  if (node !== null && typeof node === 'object' && 'props' in node) {
    return textOf((node as { props: { children?: unknown } }).props.children);
  }
  return '';
}

describe('LLMSettings model management (M2)', () => {
  it('renders the configured default model as editable (not a baked span)', async () => {
    stubFetch((url) => {
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      return { ok: true, status: 200, json: async () => settings() };
    });
    const tree = await render();
    const input = tree.root.findByProps({ 'aria-label': 'NVIDIA NIM default model' });
    expect(input.props.value).toBe('nvidia/fleet-model');
    expect(input.type).toBe('input');
  });

  it('saves an edited model id through to the backend', async () => {
    const seen: string[] = [];
    stubFetch((url, init) => {
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      if (url.includes('/api/llm/settings') && init?.method === 'POST') {
        seen.push(String(init.body));
        return { ok: true, status: 200, json: async () => ({ ok: true }) };
      }
      return { ok: true, status: 200, json: async () => settings() };
    });
    const tree = await render();
    const input = tree.root.findByProps({ 'aria-label': 'NVIDIA NIM default model' });
    await act(async () => {
      input.props.onChange({ target: { value: 'nvidia/new-model' } });
    });
    const buttons = tree.root.findAllByType('button');
    const save = buttons.find((b) => textOf(b.props.children).includes('Save'));
    await act(async () => {
      await save!.props.onClick();
    });
    expect(seen).toHaveLength(1);
    expect(JSON.parse(seen[0]).providers.nvidia.default_model).toBe('nvidia/new-model');
  });

  it('shows never-here until a provider completes a call', async () => {
    stubFetch((url) => {
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      return { ok: true, status: 200, json: async () => settings() };
    });
    const tree = await render();
    expect(JSON.stringify(tree.toJSON())).toContain('never here');
  });

  it('kill-switch toggle flips and persists via save', async () => {
    const seen: string[] = [];
    stubFetch((url, init) => {
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      if (url.includes('/api/llm/settings') && init?.method === 'POST') {
        seen.push(String(init.body));
        return { ok: true, status: 200, json: async () => ({ ok: true }) };
      }
      return { ok: true, status: 200, json: async () => settings() };
    });
    const tree = await render();
    expect(JSON.stringify(tree.toJSON()).includes('KILLED')).toBe(false);
    await act(async () => {
      tree.root.findByProps({ 'aria-label': 'Toggle DSML tool-invocation fallback' }).props.onClick();
    });
    expect(JSON.stringify(tree.toJSON())).toContain('KILLED');
    await act(async () => {
      const save = tree.root.findAllByType('button').find((b) => textOf(b.props.children).includes('Save'));
      await save!.props.onClick();
    });
    expect(seen).toHaveLength(1);
    expect(JSON.parse(seen[0]).disable_dsml_fallback).toBe(true);
  });
});

describe('LLMSettings labels + save-refusal mapping (Wave-2)', () => {
  it('associates every label with its input', async () => {
    stubFetch((url) => {
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      return { ok: true, status: 200, json: async () => settings() };
    });
    const tree = await render();
    const labels = tree.root.findAllByType('label');
    expect(labels.length).toBeGreaterThan(0);
    for (const label of labels) {
      const htmlFor = (label.props as { htmlFor?: string }).htmlFor;
      expect(htmlFor, `label "${textOf(label.props.children)}" has no htmlFor`).toBeTruthy();
      const targets = tree.root.findAll(
        (n) => (n.props as { id?: string }).id === htmlFor && (n.type === 'input' || n.type === 'select'),
      );
      expect(targets.length, `no input/select with id "${htmlFor}"`).toBeGreaterThan(0);
    }
  });

  it('maps a provider save refusal adjacent to the offending group, verbatim', async () => {
    const refusal = 'unknown provider "nvidia": backend refused to persist';
    stubFetch((url, init) => {
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      if (url.includes('/api/llm/settings') && init?.method === 'POST') {
        return { ok: false, status: 400, text: async () => refusal, json: async () => ({}) };
      }
      return { ok: true, status: 200, json: async () => settings() };
    });
    const tree = await render();
    await act(async () => {
      const save = tree.root.findAllByType('button').find((b) => textOf(b.props.children).includes('Save'));
      await save!.props.onClick();
    });
    const adjacent = tree.root.findByProps({ 'data-testid': 'llm-save-error-nvidia' });
    expect(textOf(adjacent)).toContain(refusal);
    expect(textOf(tree.root.findByProps({ 'data-testid': 'llm-save-status' }))).toContain('provider NVIDIA NIM');
  });

  it('labels an unrecognized refusal as a general section error', async () => {
    stubFetch((url, init) => {
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      if (url.includes('/api/llm/settings') && init?.method === 'POST') {
        return { ok: false, status: 500, text: async () => 'disk full on ledger', json: async () => ({}) };
      }
      return { ok: true, status: 200, json: async () => settings() };
    });
    const tree = await render();
    await act(async () => {
      const save = tree.root.findAllByType('button').find((b) => textOf(b.props.children).includes('Save'));
      await save!.props.onClick();
    });
    const banner = textOf(tree.root.findByProps({ 'data-testid': 'llm-save-status' }));
    expect(banner).toContain('general');
    expect(banner).toContain('disk full on ledger');
    expect(tree.root.findAll((n) => String((n.props as { 'data-testid'?: string })['data-testid'] ?? '').startsWith('llm-save-error-'))).toHaveLength(0);
  });

  it('keeps the dirty gate: edit marks unsaved, save clears it', async () => {
    stubFetch((url, init) => {
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      if (url.includes('/api/llm/settings') && init?.method === 'POST') {
        return { ok: true, status: 200, json: async () => ({ ok: true }), text: async () => '' };
      }
      return { ok: true, status: 200, json: async () => settings() };
    });
    const tree = await render();
    expect(JSON.stringify(tree.toJSON())).not.toContain('Unsaved changes');
    await act(async () => {
      tree.root.findByProps({ 'aria-label': 'NVIDIA NIM default model' }).props.onChange({ target: { value: 'nvidia/other' } });
    });
    expect(JSON.stringify(tree.toJSON())).toContain('Unsaved changes');
    await act(async () => {
      const save = tree.root.findAllByType('button').find((b) => textOf(b.props.children).includes('Save'));
      expect(save!.props.disabled).toBe(false);
      await save!.props.onClick();
    });
    expect(JSON.stringify(tree.toJSON())).toContain('persisted successfully');
    expect(JSON.stringify(tree.toJSON())).not.toContain('Unsaved changes');
  });

  it('disables Save while a save is in flight', async () => {
    let release!: () => void;
    const gate = new Promise((resolve) => {
      release = () => resolve({ ok: true, status: 200, json: async () => ({ ok: true }), text: async () => '' });
    });
    stubFetch((url, init) => {
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      if (url.includes('/api/llm/settings') && init?.method === 'POST') return gate;
      return { ok: true, status: 200, json: async () => settings() };
    });
    const tree = await render();
    let pending: Promise<void> | undefined;
    act(() => {
      const save = tree.root.findAllByType('button').find((b) => textOf(b.props.children).includes('Save'));
      pending = save!.props.onClick() as Promise<void>;
    });
    expect(tree.root.findAllByType('button').find((b) => textOf(b.props.children).includes('Save'))!.props.disabled).toBe(true);
    await act(async () => {
      release();
      await pending;
    });
    expect(tree.root.findAllByType('button').find((b) => textOf(b.props.children).includes('Save'))!.props.disabled).toBe(false);
  });
});
