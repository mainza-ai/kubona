import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { CognitiveMesh } from './CognitiveMesh';

// Real-browser port of CognitiveMesh.test.tsx. The mesh fans out to the real
// fetch rail, so instead of mocking `@/lib/api` each test stubs global
// `fetch` by URL/method and asserts against the rendered DOM. Unexpected
// URLs are recorded and asserted empty so a new rail call cannot pass
// silently as a caught-and-swallowed failure.

const SPEND = {
  cap_usd: 10, spent_usd: 4, remaining_usd: 6, unmetered_count: 0,
  by_provider: { openai: 4 }, active_provider: 'openai',
  local_bypass: false, heuristic: true, currency: 'USD', track_only: false,
};

type Handler = (url: string, init: RequestInit) => { body?: unknown; status?: number; ok?: boolean } | undefined;

function stubMesh(handler: Handler) {
  const unexpected: string[] = [];
  vi.stubGlobal('fetch', async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = String(input);
    const result = handler(url, init);
    if (!result) {
      unexpected.push(`${init.method ?? 'GET'} ${url}`);
      return { ok: false, status: 404, json: async () => ({ code: 'unexpected', message: `unexpected fetch ${url}` }) };
    }
    return {
      ok: result.ok ?? true,
      status: result.status ?? 200,
      json: async () => result.body ?? {},
    };
  });
  return unexpected;
}

const waitForText = async (screen: { container: HTMLElement }, needle: string) => {
  await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain(needle);
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('CognitiveMesh in a real browser', () => {
  it('renders radar endpoints and agent roster cleanly', async () => {
    const unexpected = stubMesh((url) => {
      if (url.includes('/api/radar')) {
        return { body: { radar_endpoints: [{ id: 'ep-1', service: 'Local Ollama', port: 11434, listening: true }] } };
      }
      if (url.includes('/api/llm/settings')) return { body: { air_gapped: true } };
      if (url.includes('/api/cost/summary')) return { body: SPEND };
      if (url.includes('/api/agents/custom')) {
        return {
          body: {
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
          },
        };
      }
      if (url.includes('/api/agents')) return { body: { roles: [] } };
      return undefined;
    });

    const screen = await render(<CognitiveMesh />);
    await waitForText(screen, 'Theory Scout');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('Sovereign Cognitive Mesh');
    expect(text).toContain('Local Ollama');
    expect(text).toContain('Theory Scout');
    expect(unexpected).toEqual([]);
  });

  it('renders routing decision safely when modalities_supported is null', async () => {
    const unexpected = stubMesh((url) => {
      if (url.includes('/api/radar')) return { body: { radar_endpoints: [] } };
      if (url.includes('/api/llm/settings')) return { body: { air_gapped: false } };
      if (url.includes('/api/cost/summary')) return { body: SPEND };
      if (url.includes('/api/agents/custom')) return { body: { agents: [] } };
      if (url.includes('/api/agents')) return { body: { roles: [] } };
      if (url.includes('/api/mesh/route')) {
        return {
          body: {
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
          },
        };
      }
      return undefined;
    });

    const screen = await render(<CognitiveMesh />);
    // Real click on the route evaluator — no synthetic handler invocation.
    await screen.getByRole('button', { name: /Evaluate Route/ }).click();
    await waitForText(screen, 'Selected Model:');

    const text = screen.container.textContent ?? '';
    expect(text).toContain('Selected Model:');
    expect(text).toContain('nemotron-3.5');
    expect(text).toContain('CLOUD ESCALATION');
    expect(text).toContain('Modalities:');
    expect(text).toContain('general');
    expect(unexpected).toEqual([]);
  });

  it('renders the server-sealed spend strip with local-bypass cue', async () => {
    const unexpected = stubMesh((url) => {
      if (url.includes('/api/radar')) return { body: { radar_endpoints: [] } };
      if (url.includes('/api/llm/settings')) return { body: { air_gapped: true } };
      if (url.includes('/api/agents/custom')) return { body: { agents: [] } };
      if (url.includes('/api/agents')) return { body: { roles: [] } };
      if (url.includes('/api/cost/summary')) {
        return {
          body: {
            cap_usd: 0, spent_usd: 0, remaining_usd: null, unmetered_count: 0,
            by_provider: {}, active_provider: 'strata',
            local_bypass: true, heuristic: true, currency: 'USD', track_only: true,
          },
        };
      }
      return undefined;
    });

    const screen = await render(<CognitiveMesh />);
    await waitForText(screen, 'local inference $0');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('LLM spend — server-sealed');
    expect(text).toContain('local inference $0');
    expect(text).toContain('unmetered (active: strata)');
    expect(text).toContain('no cap (track-only)');
    expect(unexpected).toEqual([]);
  });

  it('shows the spend refusal instead of inventing figures', async () => {
    const unexpected = stubMesh((url) => {
      if (url.includes('/api/radar')) return { body: { radar_endpoints: [] } };
      if (url.includes('/api/llm/settings')) return { body: { air_gapped: true } };
      if (url.includes('/api/agents/custom')) return { body: { agents: [] } };
      if (url.includes('/api/agents')) return { body: { roles: [] } };
      if (url.includes('/api/cost/summary')) {
        return { ok: false, status: 503, body: { code: 'unavailable', message: 'spend store unreachable' } };
      }
      return undefined;
    });

    const screen = await render(<CognitiveMesh />);
    await waitForText(screen, 'Spend refused');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('LLM spend — server-sealed');
    expect(text).toContain('Spend refused');
    expect(text).not.toContain('local inference $0');
    expect(unexpected).toEqual([]);
  });
});
