import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { LLMSettings } from './LLMSettings';

afterEach(() => {
  vi.unstubAllGlobals();
});

function settings(overrides?: Partial<ReturnType<typeof baseSettings>>) {
  return { ...baseSettings(), ...overrides };
}

interface TestProviderSeed {
  type: string;
  name: string;
  base_url: string;
  default_model: string;
  enabled: boolean;
  is_local: boolean;
  timeout_sec: number;
  has_key?: boolean;
  secret_source?: string;
  key_status?: string;
  wire?: string;
}

interface TestRoleSeed {
  role: string;
  provider: string;
  model: string;
}

function baseSettings(): {
  active_provider: string;
  active_model: string;
  air_gapped: boolean;
  temperature: number;
  max_tokens: number;
  providers: Record<string, TestProviderSeed>;
  role_assignments: Record<string, TestRoleSeed>;
} {
  return {
    active_provider: 'nvidia',
    active_model: 'nvidia/fleet-model',
    air_gapped: false,
    temperature: 0.2,
    max_tokens: 0,
    providers: {
      nvidia: {
        type: 'nvidia',
        name: 'NVIDIA NIM',
        base_url: 'https://integrate.api.nvidia.com/v1',
        default_model: 'nvidia/fleet-model',
        enabled: true,
        is_local: false,
        timeout_sec: 120,
        has_key: true,
        secret_source: 'sealed_db',
      },
      omlx: {
        type: 'omlx',
        name: 'oMLX',
        base_url: 'http://127.0.0.1:8787/v1',
        default_model: '',
        enabled: true,
        is_local: true,
        timeout_sec: 30,
      },
    },
    role_assignments: {
      copilot: { role: 'copilot', provider: 'nvidia', model: 'nvidia/fleet-model' },
    },
  };
}

function costSummaryPayload() {
  return {
    spent_usd: 0,
    cap_usd: 0,
    remaining_usd: 0,
    active_provider: 'nvidia',
    currency: 'USD',
    unlimited: true,
    local_bypass: true,
    unmetered_count: 0,
    by_provider: {},
    by_model: {},
  };
}

function stubFetch(handler: (url: string, init?: RequestInit) => unknown) {
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    if (url.includes('/api/cost/summary')) {
      return { ok: true, status: 200, json: async () => costSummaryPayload() } as Response;
    }
    return handler(url, init) as Response;
  });
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
    expect(JSON.stringify(tree.toJSON())).toContain('applied · LLM settings persisted');
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

describe('LLMSettings first-load skeleton (Wave-3)', () => {
  it('renders an honest loading skeleton while settings load', async () => {
    stubFetch(() => new Promise(() => undefined));
    let tree: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      tree = TestRenderer.create(<LLMSettings />);
    });
    expect(tree!.root.findByProps({ 'data-testid': 'llm-settings-skeleton' })).toBeDefined();
    expect(JSON.stringify(tree!.toJSON())).toContain('Loading Sovereign Intelligence Configuration');
  });

  it('hides the skeleton once the form loads', async () => {
    stubFetch((url) => {
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      return { ok: true, status: 200, json: async () => settings() };
    });
    const tree = await render();
    expect(tree.root.findAllByProps({ 'data-testid': 'llm-settings-skeleton' })).toHaveLength(0);
    expect(JSON.stringify(tree.toJSON())).toContain('Sovereign Intelligence Configuration');
  });
});

describe('LLMSettings Phase-0 credential truth, discovery errors & 9-role catalog', () => {
  it('shows (not set) even when a cloud provider is enabled if has_key is false', async () => {
    const data = settings();
    data.providers.nvidia = { ...data.providers.nvidia, enabled: true, has_key: false, secret_source: 'none' };
    stubFetch((url) => {
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      return { ok: true, status: 200, json: async () => data };
    });
    const tree = await render();
    const json = JSON.stringify(tree.toJSON());
    expect(json).toContain('(not set)');
    expect(json).not.toContain('(sealed ●●●)');
  });

  it('warns on unsealed plaintext_db storage and calls DELETE when Remove key is clicked', async () => {
    const data = settings();
    data.providers.nvidia = { ...data.providers.nvidia, has_key: true, secret_source: 'plaintext_db' };
    const afterClear = settings();
    afterClear.providers.nvidia = { ...afterClear.providers.nvidia, has_key: false, secret_source: 'none' };
    const calls: Array<{ url: string; method?: string }> = [];
    stubFetch((url, init) => {
      calls.push({ url, method: init?.method });
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      if (url.includes('/api/llm/providers/nvidia/key') && init?.method === 'DELETE') {
        const body = { ok: true, settings: afterClear };
        return {
          ok: true,
          status: 200,
          text: async () => JSON.stringify(body),
          json: async () => body,
        };
      }
      return { ok: true, status: 200, json: async () => data };
    });
    const tree = await render();
    expect(JSON.stringify(tree.toJSON())).toContain('(unsealed DB ●●●)');
    expect(JSON.stringify(tree.toJSON())).toContain('KUBONA_SECRET_KEY');
    const removeBtn = tree.root.findAllByType('button').find((b) => textOf(b.props.children).includes('Remove key'))!;
    await act(async () => {
      await removeBtn.props.onClick();
    });
    expect(calls.some((c) => c.url.includes('/api/llm/providers/nvidia/key') && c.method === 'DELETE')).toBe(true);
    expect(JSON.stringify(tree.toJSON())).toContain('(not set)');
  });

  it('surfaces inline modelErrors when model discovery fails', async () => {
    stubFetch((url) => {
      if (url.includes('/api/llm/models?provider=nvidia')) {
        return { ok: false, status: 502, text: async () => 'upstream dial timeout', json: async () => ({}) };
      }
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      return { ok: true, status: 200, json: async () => settings() };
    });
    const tree = await render();
    const errBox = tree.root.findByProps({ 'data-testid': 'llm-model-error-nvidia' });
    expect(textOf(errBox)).toContain('upstream dial timeout');
  });

  it('renders all 9 runtime roles grouped into Interactive Assistant, Research Tribunal, and Autonomous Loops', async () => {
    const roles = [
      'copilot',
      'epistemic-scribe',
      'ansatz-scout',
      'disconfirmation-officer',
      'device-auditor',
      'codesign',
      'coder',
      'critic',
      'supervisor',
    ];
    const data = settings({
      role_assignments: Object.fromEntries(roles.map((r) => [r, { role: r, provider: 'omlx', model: 'm1' }])),
    });
    stubFetch((url) => {
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      return { ok: true, status: 200, json: async () => data };
    });
    const tree = await render();
    const json = JSON.stringify(tree.toJSON());
    expect(json).toContain('Interactive Assistant');
    expect(json).toContain('Research Tribunal');
    expect(json).toContain('Autonomous Loops');
    for (const r of roles) {
      expect(tree.root.findByProps({ 'aria-label': `${r} provider` })).toBeDefined();
    }
  });
});

describe('LLMSettings Phase-2/3 discovery, verify race guard, failover, profiles & benchmark overrides', () => {
  it('verifies provider key via free /models and discards stale check_generation responses', async () => {
    let resolveFirst!: (val: unknown) => void;
    let resolveSecond!: (val: unknown) => void;
    let verifyCount = 0;

    stubFetch((url, init) => {
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      if (url.includes('/api/llm/providers/nvidia/verify') && init?.method === 'POST') {
        verifyCount += 1;
        const body = JSON.parse(String(init.body));
        if (verifyCount === 1) {
          const payload = {
            ok: false,
            verification: {
              ok: false,
              provider: 'nvidia',
              wire: 'openai-chat',
              key_status: 'invalid',
              fault: 'wrong_key',
              error: 'stale bad key',
              latency_ms: 15,
              models: [],
              check_generation: body.check_generation,
            },
          };
          return new Promise((r) => {
            resolveFirst = () =>
              r({
                ok: true,
                status: 200,
                text: async () => JSON.stringify(payload),
                json: async () => payload,
              });
          });
        }
        const okPayload = {
          ok: true,
          verification: {
            ok: true,
            provider: 'nvidia',
            wire: 'openai-chat',
            key_status: 'valid',
            latency_ms: 42,
            models: [{ id: 'meta/llama-3.3-70b', provider: 'nvidia' }],
            check_generation: body.check_generation,
          },
          settings: settings({
            providers: {
              ...settings().providers,
              nvidia: { ...settings().providers.nvidia, key_status: 'valid', has_key: true },
            },
          }),
        };
        return new Promise((r) => {
          resolveSecond = () =>
            r({
              ok: true,
              status: 200,
              text: async () => JSON.stringify(okPayload),
              json: async () => okPayload,
            });
        });
      }
      return { ok: true, status: 200, json: async () => settings() };
    });

    const tree = await render();
    const verifyBtn = tree.root.findByProps({ 'aria-label': 'Verify NVIDIA NIM' });

    let p1!: Promise<void>;
    let p2!: Promise<void>;
    act(() => {
      p1 = verifyBtn.props.onClick() as Promise<void>;
      p2 = verifyBtn.props.onClick() as Promise<void>;
    });
    await act(async () => {
      resolveSecond(undefined);
      await p2;
    });
    await act(async () => {
      resolveFirst(undefined);
      await p1;
    });

    const badgeText = textOf(tree.root.findByProps({ 'data-testid': 'llm-verify-result-nvidia' }));
    expect(badgeText).toContain('Verified');
    expect(badgeText).toContain('42 ms');
    expect(badgeText).not.toContain('stale bad key');
  });

  it('discovers local servers with Ollama context warning and attaches with one click', async () => {
    const baseData = settings({
      providers: {
        ...settings().providers,
        ollama: {
          type: 'ollama',
          name: 'Ollama',
          base_url: 'http://127.0.0.1:11434',
          default_model: '',
          enabled: false,
          is_local: true,
          timeout_sec: 120,
        },
      },
    });
    stubFetch((url) => {
      if (url.includes('/api/llm/models')) return { ok: true, status: 200, json: async () => ({ models: [] }) };
      if (url.includes('/api/llm/discover')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            servers: [
              {
                provider: 'ollama',
                name: 'Ollama Local Engine',
                base_url: 'http://127.0.0.1:11434',
                wire: 'ollama-chat',
                port: 11434,
                models: ['qwen2.5:14b'],
                context_window: 4096,
                context_warning: 'Ollama num_ctx=4096 detected (< 8192); consider raising num_ctx.',
                source: 'http',
              },
            ],
            ports_scanned: [8787, 11434],
            checked_at: '2026-11-11T00:00:00Z',
          }),
        };
      }
      return { ok: true, status: 200, json: async () => baseData };
    });

    const tree = await render();
    const scanBtn = tree.root
      .findAllByType('button')
      .find((b) => textOf(b.props.children).includes('Scan local servers'))!;
    await act(async () => {
      await scanBtn.props.onClick();
    });

    const card = tree.root.findByProps({ 'data-testid': 'discovered-server-ollama' });
    const cardText = textOf(card);
    expect(cardText).toContain('qwen2.5:14b');
    expect(cardText).toContain('num_ctx=4096');

    const attachBtn = card.findAllByType('button').find((b) => textOf(b.props.children).includes('Attach'))!;
    await act(async () => {
      attachBtn.props.onClick();
    });
    expect(tree.root.findByProps({ 'aria-label': 'Ollama default model' }).props.value).toBe('qwen2.5:14b');
    expect(tree.root.findByProps({ 'aria-label': 'Toggle Ollama' }).props.children).toBe('ENABLED');
  });

  it('edits BaseURL/Wire, configures fallback hops and per-role params, warns on mono-model tribunal & unpriced loops, applies profiles, and runs explicit benchmark', async () => {
    const savedPayloads: Record<string, unknown>[] = [];
    const testPayloads: Record<string, unknown>[] = [];
    const data = settings({
      air_gapped: false,
      providers: {
        ...settings().providers,
        omlx: {
          type: 'omlx',
          name: 'oMLX',
          base_url: 'http://127.0.0.1:8787/v1',
          default_model: 'local-qwen',
          enabled: true,
          is_local: true,
          timeout_sec: 120,
        },
        nvidia: {
          ...settings().providers.nvidia,
          enabled: true,
          has_key: true,
          key_status: 'valid',
          default_model: 'nvidia/unpriced-model',
        },
      },
      role_assignments: {
        copilot: { role: 'copilot', provider: 'nvidia', model: 'nvidia/unpriced-model' },
        'ansatz-scout': { role: 'ansatz-scout', provider: 'nvidia', model: 'nvidia/unpriced-model' },
        'disconfirmation-officer': { role: 'disconfirmation-officer', provider: 'nvidia', model: 'nvidia/unpriced-model' },
        'epistemic-scribe': { role: 'epistemic-scribe', provider: 'omlx', model: '' },
      },
    });

    stubFetch((url, init) => {
      if (url.includes('/api/llm/models')) {
        if (url.includes('provider=nvidia')) {
          return {
            ok: true,
            status: 200,
            json: async () => ({
              models: [{ id: 'nvidia/unpriced-model', provider: 'nvidia', priced: false }],
            }),
          };
        }
        return {
          ok: true,
          status: 200,
          json: async () => ({ models: [{ id: 'local-qwen', provider: 'omlx' }] }),
        };
      }
      if (url.includes('/api/llm/settings') && init?.method === 'POST') {
        savedPayloads.push(JSON.parse(String(init.body)));
        return { ok: true, status: 200, json: async () => ({ ok: true }) };
      }
      if (url.includes('/api/llm/test') && init?.method === 'POST') {
        testPayloads.push(JSON.parse(String(init.body)));
        return {
          ok: true,
          status: 200,
          json: async () => ({
            success: true,
            provider: 'nvidia',
            wire: 'openai-chat',
            model: 'nvidia/bench-override',
            reply: 'pong',
            duration_ms: 19,
            attempt_count: 2,
            cost_usd: 0.00012,
          }),
        };
      }
      return { ok: true, status: 200, json: async () => data };
    });

    const tree = await render();

    // Mono-model tribunal advisory + unpriced non-local warning
    expect(textOf(tree.root.findByProps({ 'data-testid': 'mono-model-tribunal-warning' }))).toContain('Ansatz Scout and Disconfirmation Officer share nvidia:nvidia/unpriced-model');
    expect(JSON.stringify(tree.toJSON())).toContain('Unpriced · loops blocked');

    // Differentiate disconfirmation-officer to clear warning
    await act(async () => {
      tree.root.findByProps({ 'aria-label': 'disconfirmation-officer provider' }).props.onChange({ target: { value: 'omlx' } });
      tree.root.findByProps({ 'aria-label': 'disconfirmation-officer model' }).props.onChange({ target: { value: 'local-qwen' } });
    });
    expect(tree.root.findAllByProps({ 'data-testid': 'mono-model-tribunal-warning' })).toHaveLength(0);

    // Edit BaseURL and Wire for NVIDIA NIM
    await act(async () => {
      tree.root.findByProps({ 'aria-label': 'NVIDIA NIM base URL' }).props.onChange({ target: { value: 'http://127.0.0.1:9001/v1' } });
      tree.root.findByProps({ 'aria-label': 'NVIDIA NIM wire protocol' }).props.onChange({ target: { value: 'openai-responses' } });
    });

    // Open copilot role options, set per-role temperature/seed/reasoning_effort and add fallback
    await act(async () => {
      tree.root.findByProps({ 'aria-label': 'Configure copilot sampling and fallbacks' }).props.onClick();
    });
    await act(async () => {
      tree.root.findByProps({ 'aria-label': 'copilot temperature' }).props.onChange({ target: { value: '0.15' } });
      tree.root.findByProps({ 'aria-label': 'copilot seed' }).props.onChange({ target: { value: '7' } });
      tree.root.findByProps({ 'aria-label': 'copilot reasoning effort' }).props.onChange({ target: { value: 'high' } });
      const addFb = tree.root.findAllByType('button').find((b) => textOf(b.props.children).includes('Add fallback'))!;
      addFb.props.onClick();
    });

    const saveBtn = tree.root.findAllByType('button').find((b) => textOf(b.props.children).includes('Save'))!;
    await act(async () => {
      await saveBtn.props.onClick();
    });

    expect(savedPayloads).toHaveLength(1);
    const saved = savedPayloads[0] as {
      providers: Record<string, { base_url: string; wire?: string }>;
      role_assignments: Record<string, { temperature?: number; seed?: number; reasoning_effort?: string; fallback?: Array<{ provider: string; model: string }> }>;
    };
    expect(saved.providers.nvidia.base_url).toBe('http://127.0.0.1:9001/v1');
    expect(saved.providers.nvidia.wire).toBe('openai-responses');
    expect(saved.role_assignments.copilot.temperature).toBe(0.15);
    expect(saved.role_assignments.copilot.seed).toBe(7);
    expect(saved.role_assignments.copilot.reasoning_effort).toBe('high');
    expect(saved.role_assignments.copilot.fallback?.length).toBe(1);

    // Explicit benchmark provider+model override
    await act(async () => {
      tree.root.findByProps({ 'aria-label': 'Benchmark provider override' }).props.onChange({ target: { value: 'nvidia' } });
      tree.root.findByProps({ 'aria-label': 'Benchmark model override' }).props.onChange({ target: { value: 'nvidia/bench-override' } });
    });
    const benchBtn = tree.root.findAllByType('button').find((b) => textOf(b.props.children).includes('Test Model'))!;
    await act(async () => {
      await benchBtn.props.onClick();
    });
    expect(testPayloads[0]).toMatchObject({ provider: 'nvidia', model: 'nvidia/bench-override' });
    const json = JSON.stringify(tree.toJSON());
    expect(json).toContain('openai-chat');
    expect(json).toContain('$0.00012');

    // Fill unassigned roles + Sovereign Local profile
    await act(async () => {
      const fillBtn = tree.root.findAllByType('button').find((b) => textOf(b.props.children).includes('Fill unassigned roles'))!;
      fillBtn.props.onClick();
    });
    expect(tree.root.findByProps({ 'aria-label': 'epistemic-scribe model' }).props.value).toBe('nvidia/fleet-model');

    await act(async () => {
      const sovBtn = tree.root.findAllByType('button').find((b) => textOf(b.props.children).includes('Sovereign Local'))!;
      sovBtn.props.onClick();
    });
    expect(tree.root.findByProps({ 'aria-label': 'copilot provider' }).props.value).toBe('omlx');
    expect(tree.root.findByProps({ 'aria-label': 'copilot model' }).props.value).toBe('local-qwen');
  });
});
