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
});
