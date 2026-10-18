import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { CognitiveMesh } from './CognitiveMesh';

afterEach(() => {
  vi.unstubAllGlobals();
});

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(textOf).join(' ');
  if (node !== null && typeof node === 'object') {
    const obj = node as { children?: unknown; props?: { children?: unknown } };
    if ('children' in obj && obj.children != null) {
      return textOf(obj.children);
    }
    if ('props' in obj && obj.props?.children != null) {
      return textOf(obj.props.children);
    }
  }
  return '';
}

function stubFetch(handler: (url: string, init?: RequestInit) => unknown) {
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => handler(url, init) as Response);
}

describe('CognitiveMesh', () => {
  it('renders radar endpoints and agent roster cleanly', async () => {
    stubFetch((url) => {
      if (url.includes('/api/radar')) {
        return { ok: true, json: async () => ({ radar_endpoints: [{ id: 'ep-1', service: 'Local Ollama', port: 11434, listening: true }] }) };
      }
      if (url.includes('/api/llm/settings')) {
        return { ok: true, json: async () => ({ air_gapped: true }) };
      }
      if (url.includes('/api/agents/custom')) {
        return {
          ok: true,
          json: async () => ({
            agents: [
              {
                id: 'agent-1',
                name: 'Theory Scout',
                role: 'theorist',
                avatar_emoji: '🔬',
                description: 'Investigates Hamiltonian symmetries',
                system_prompt: 'Test prompt',
                preferred_model: 'local/qwen2.5-coder',
                modalities: ['text'],
                is_system_agent: true,
              },
            ],
          }),
        };
      }
      throw new Error(`unexpected fetch ${url}`);
    });

    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<CognitiveMesh />);
    });

    const text = textOf(tree!.toJSON());
    expect(text).toContain('Sovereign Cognitive Mesh');
    expect(text).toContain('Local Ollama');
    expect(text).toContain('Theory Scout');
  });

  it('renders routing decision safely when modalities_supported is null', async () => {
    stubFetch((url) => {
      if (url.includes('/api/radar')) return { ok: true, json: async () => ({ radar_endpoints: [] }) };
      if (url.includes('/api/llm/settings')) return { ok: true, json: async () => ({ air_gapped: false }) };
      if (url.includes('/api/agents/custom')) return { ok: true, json: async () => ({ agents: [] }) };
      if (url.includes('/api/mesh/route')) {
        return {
          ok: true,
          json: async () => ({
            ok: true,
            decision: {
              provider: 'nvidia',
              model: 'nemotron-3.5',
              is_local: false,
              air_gap_enforced: false,
              estimated_cost_usd: 0.045,
              explanation: 'cloud model requested without budget backing',
              modalities_supported: null, // Critical edge case test
            },
          }),
        };
      }
      throw new Error(`unexpected fetch ${url}`);
    });

    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<CognitiveMesh />);
    });

    // Find the test route button
    const buttons = tree!.root.findAllByType('button');
    const routeBtn = buttons.find((b) => textOf(b.props.children).includes('Evaluate Route'));
    expect(routeBtn).toBeDefined();

    await act(async () => {
      await routeBtn!.props.onClick();
    });

    const text = textOf(tree!.toJSON());
    expect(text).toContain('Selected Model:');
    expect(text).toContain('nemotron-3.5');
    expect(text).toContain('CLOUD ESCALATION');
    expect(text).toContain('Modalities:');
    expect(text).toContain('general');
  });
});
