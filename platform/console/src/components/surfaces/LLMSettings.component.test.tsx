import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { LLMSettings } from './LLMSettings';
import { setAuthPhase } from '@/domain/auth-state';

afterEach(() => {
  vi.unstubAllGlobals();
});

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

function settings(overrides?: Partial<ReturnType<typeof baseSettings>>) {
  return { ...baseSettings(), ...overrides };
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

function json(body: unknown) {
  return { ok: true, status: 200, json: async () => body };
}

function refusal(status: number, body: string) {
  return { ok: false, status, text: async () => body, json: async () => ({}) };
}

type FetchHandler = (url: string, init?: RequestInit) => unknown;

/**
 * Every route is answered at fetch level: the surface talks to the real
 * transport wrappers, so mocking `@/lib/api` would bypass the request and
 * refusal mapping this suite is here to verify.
 */
function installFetch(handler: FetchHandler) {
  const calls: Array<{ url: string; method: string; body?: unknown }> = [];
  vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({ url, method, body: init?.body });
    if (url.includes('/api/cost/summary')) return json(costSummaryPayload());
    return handler(url, init);
  });
  return calls;
}

async function mount() {
  setAuthPhase('authenticated');
  return render(<LLMSettings />);
}

function bodyText(screen: { container: HTMLElement }): string {
  return screen.container.textContent ?? '';
}

function settingsRoutes(data: unknown, init?: (url: string) => unknown) {
  return (url: string) => {
    const handled = init?.(url);
    if (handled !== undefined) return handled;
    if (url.includes('/api/llm/models')) return json({ models: [] });
    return json(data);
  };
}

describe('LLMSettings model management (M2) in a real browser', () => {
  it('renders the configured default model as editable (not a baked span)', async () => {
    installFetch(settingsRoutes(settings()));

    const screen = await mount();
    const input = screen.getByLabelText('NVIDIA NIM default model');
    await expect.element(input).toBeVisible();
    expect(input.element().tagName).toBe('INPUT');
    expect((input.element() as HTMLInputElement).value).toBe('nvidia/fleet-model');
  });

  it('saves an edited model id through to the backend', async () => {
    const seen: string[] = [];
    installFetch((url, init) => {
      if (url.includes('/api/llm/models')) return json({ models: [] });
      if (url.includes('/api/llm/settings') && init?.method === 'POST') {
        seen.push(String(init.body));
        return json({ ok: true });
      }
      return json(settings());
    });

    const screen = await mount();
    await screen.getByLabelText('NVIDIA NIM default model').fill('nvidia/new-model');
    await screen.getByRole('button', { name: /save settings/i }).click();

    await expect.poll(() => seen.length).toBe(1);
    expect(JSON.parse(seen[0]).providers.nvidia.default_model).toBe('nvidia/new-model');
  });

  it('shows never-here until a provider completes a call', async () => {
    installFetch(settingsRoutes(settings()));

    const screen = await mount();
    await expect.element(screen.getByLabelText('NVIDIA NIM default model')).toBeVisible();
    expect(bodyText(screen)).toContain('never here');
  });

  it('kill-switch toggle flips and persists via save', async () => {
    const seen: string[] = [];
    installFetch((url, init) => {
      if (url.includes('/api/llm/models')) return json({ models: [] });
      if (url.includes('/api/llm/settings') && init?.method === 'POST') {
        seen.push(String(init.body));
        return json({ ok: true });
      }
      return json(settings());
    });

    const screen = await mount();
    const toggle = screen.getByLabelText('Toggle DSML tool-invocation fallback');
    await expect.element(toggle).toBeVisible();
    expect(bodyText(screen)).not.toContain('KILLED');

    await toggle.click();
    await expect.poll(() => bodyText(screen)).toContain('KILLED');

    await screen.getByRole('button', { name: /save settings/i }).click();
    await expect.poll(() => seen.length).toBe(1);
    expect(JSON.parse(seen[0]).disable_dsml_fallback).toBe(true);
  });
});

describe('LLMSettings labels + save-refusal mapping (Wave-2) in a real browser', () => {
  it('associates every label with its input', async () => {
    installFetch(settingsRoutes(settings()));

    const screen = await mount();
    await expect.element(screen.getByLabelText('NVIDIA NIM default model')).toBeVisible();

    const labels = Array.from(screen.container.querySelectorAll('label'));
    expect(labels.length).toBeGreaterThan(0);
    for (const label of labels) {
      const htmlFor = label.getAttribute('for');
      expect(htmlFor, `label "${label.textContent}" has no htmlFor`).toBeTruthy();
      const target = screen.container.querySelector(`#${CSS.escape(htmlFor!)}`);
      expect(target, `no element with id "${htmlFor}"`).not.toBeNull();
      expect(['INPUT', 'SELECT']).toContain(target!.tagName);
    }
  });

  it('maps a provider save refusal adjacent to the offending group, verbatim', async () => {
    const message = 'unknown provider "nvidia": backend refused to persist';
    installFetch((url, init) => {
      if (url.includes('/api/llm/models')) return json({ models: [] });
      if (url.includes('/api/llm/settings') && init?.method === 'POST') {
        return refusal(400, message);
      }
      return json(settings());
    });

    const screen = await mount();
    await screen.getByRole('button', { name: /save settings/i }).click();

    const adjacent = screen.getByTestId('llm-save-error-nvidia');
    await expect.element(adjacent).toMatchTextContent(message);
    await expect
      .element(screen.getByTestId('llm-save-status'))
      .toMatchTextContent('provider NVIDIA NIM');
  });

  it('labels an unrecognized refusal as a general section error', async () => {
    installFetch((url, init) => {
      if (url.includes('/api/llm/models')) return json({ models: [] });
      if (url.includes('/api/llm/settings') && init?.method === 'POST') {
        return refusal(500, 'disk full on ledger');
      }
      return json(settings());
    });

    const screen = await mount();
    await screen.getByRole('button', { name: /save settings/i }).click();

    const banner = screen.getByTestId('llm-save-status');
    await expect.element(banner).toMatchTextContent('general');
    expect(banner.element().textContent).toContain('disk full on ledger');
    expect(screen.container.querySelectorAll('[data-testid^="llm-save-error-"]')).toHaveLength(0);
  });

  it('keeps the dirty gate: edit marks unsaved, save clears it', async () => {
    installFetch((url, init) => {
      if (url.includes('/api/llm/models')) return json({ models: [] });
      if (url.includes('/api/llm/settings') && init?.method === 'POST') {
        return { ok: true, status: 200, json: async () => ({ ok: true }), text: async () => '' };
      }
      return json(settings());
    });

    const screen = await mount();
    const model = screen.getByLabelText('NVIDIA NIM default model');
    await expect.element(model).toHaveValue('nvidia/fleet-model');
    expect(bodyText(screen)).not.toContain('Unsaved changes');

    await model.fill('nvidia/other');
    await expect.poll(() => bodyText(screen)).toContain('Unsaved changes');

    const save = screen.getByRole('button', { name: /save settings/i });
    await expect.element(save).toBeEnabled();
    await save.click();

    await expect.poll(() => bodyText(screen)).toContain('persisted successfully');
    expect(bodyText(screen)).toContain('applied · LLM settings persisted');
    expect(bodyText(screen)).not.toContain('Unsaved changes');
  });

  it('disables Save while a save is in flight', async () => {
    let release!: () => void;
    const gate = new Promise((resolve) => {
      release = () => resolve({ ok: true, status: 200, json: async () => ({ ok: true }), text: async () => '' });
    });
    installFetch((url, init) => {
      if (url.includes('/api/llm/models')) return json({ models: [] });
      if (url.includes('/api/llm/settings') && init?.method === 'POST') return gate;
      return json(settings());
    });

    const screen = await mount();
    const save = screen.getByRole('button', { name: /save settings/i });
    await expect.element(save).toBeEnabled();

    await save.click();
    await expect.element(save).toBeDisabled();

    release();
    await expect.element(save).toBeEnabled();
  });
});

describe('LLMSettings first-load skeleton (Wave-3) in a real browser', () => {
  it('renders an honest loading skeleton while settings load', async () => {
    installFetch(() => new Promise(() => undefined));

    const screen = await mount();
    await expect.element(screen.getByTestId('llm-settings-skeleton')).toBeInTheDocument();
    expect(bodyText(screen)).toContain('Loading Sovereign Intelligence Configuration');
  });

  it('hides the skeleton once the form loads', async () => {
    installFetch(settingsRoutes(settings()));

    const screen = await mount();
    await expect.element(screen.getByText('Sovereign Intelligence Configuration')).toBeVisible();
    expect(screen.container.querySelectorAll('[data-testid="llm-settings-skeleton"]')).toHaveLength(0);
  });
});

describe('LLMSettings Phase-0 credential truth, discovery errors & 9-role catalog in a real browser', () => {
  it('shows (not set) even when a cloud provider is enabled if has_key is false', async () => {
    const data = settings();
    data.providers.nvidia = { ...data.providers.nvidia, enabled: true, has_key: false, secret_source: 'none' };
    installFetch(settingsRoutes(data));

    const screen = await mount();
    await expect.element(screen.getByLabelText('NVIDIA NIM default model')).toBeVisible();
    expect(bodyText(screen)).toContain('(not set)');
    expect(bodyText(screen)).not.toContain('(sealed ●●●)');
  });

  it('warns on unsealed plaintext_db storage and calls DELETE when Remove key is clicked', async () => {
    const data = settings();
    data.providers.nvidia = { ...data.providers.nvidia, has_key: true, secret_source: 'plaintext_db' };
    const afterClear = settings();
    afterClear.providers.nvidia = { ...afterClear.providers.nvidia, has_key: false, secret_source: 'none' };

    const calls = installFetch((url, init) => {
      if (url.includes('/api/llm/models')) return json({ models: [] });
      if (url.includes('/api/llm/providers/nvidia/key') && init?.method === 'DELETE') {
        const body = { ok: true, settings: afterClear };
        return { ok: true, status: 200, text: async () => JSON.stringify(body), json: async () => body };
      }
      return json(data);
    });

    const screen = await mount();
    await expect.element(screen.getByLabelText('NVIDIA NIM default model')).toBeVisible();
    expect(bodyText(screen)).toContain('(unsealed DB ●●●)');
    expect(bodyText(screen)).toContain('KUBONA_SECRET_KEY');

    await screen.getByRole('button', { name: /remove key/i }).click();
    await expect
      .poll(() => calls.some((c) => c.url.includes('/api/llm/providers/nvidia/key') && c.method === 'DELETE'))
      .toBe(true);
    await expect.poll(() => bodyText(screen)).toContain('(not set)');
  });

  it('surfaces inline modelErrors when model discovery fails', async () => {
    installFetch((url) => {
      if (url.includes('/api/llm/models?provider=nvidia')) {
        return refusal(502, 'upstream dial timeout');
      }
      if (url.includes('/api/llm/models')) return json({ models: [] });
      return json(settings());
    });

    const screen = await mount();
    await expect.element(screen.getByTestId('llm-model-error-nvidia')).toMatchTextContent('upstream dial timeout');
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
    installFetch(settingsRoutes(data));

    const screen = await mount();
    await expect.element(screen.getByText('Interactive Assistant')).toBeVisible();
    await expect.element(screen.getByText('Research Tribunal')).toBeVisible();
    await expect.element(screen.getByText('Autonomous Loops')).toBeVisible();
    for (const r of roles) {
      await expect.element(screen.getByLabelText(`${r} provider`)).toBeVisible();
    }
  });
});

describe('LLMSettings Phase-2/3 discovery, verify race guard, failover, profiles & benchmark overrides in a real browser', () => {
  it('verifies provider key via free /models and discards stale check_generation responses', async () => {
    let verifyCount = 0;
    let resolveFirst!: () => void;
    let resolveSecond!: () => void;

    installFetch((url, init) => {
      if (url.includes('/api/llm/models')) return json({ models: [] });
      if (url.includes('/api/llm/providers/nvidia/verify') && init?.method === 'POST') {
        verifyCount += 1;
        const body = JSON.parse(String(init.body)) as { check_generation: number };
        if (verifyCount === 1) {
          const stalePayload = {
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
          return new Promise((resolve) => {
            resolveFirst = () =>
              resolve({
                ok: true,
                status: 200,
                text: async () => JSON.stringify(stalePayload),
                json: async () => stalePayload,
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
        return new Promise((resolve) => {
          resolveSecond = () =>
            resolve({
              ok: true,
              status: 200,
              text: async () => JSON.stringify(okPayload),
              json: async () => okPayload,
            });
        });
      }
      return json(settings());
    });

    const screen = await mount();
    const verify = screen.getByLabelText('Verify NVIDIA NIM');
    await expect.element(verify).toBeVisible();

    // Two clicks in one task, before React commits `disabled`: the real
    // double-invocation the generation guard exists to survive.
    const button = verify.element() as HTMLButtonElement;
    button.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    button.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    await expect.poll(() => verifyCount).toBe(2);

    // The second (newest) check resolves first; the stale first lands later.
    resolveSecond();
    await expect.poll(() => bodyText(screen)).toContain('42 ms');
    resolveFirst();
    await new Promise((resolve) => setTimeout(resolve, 0));

    const badge = screen.getByTestId('llm-verify-result-nvidia');
    expect(badge.element().textContent).toContain('Verified');
    expect(badge.element().textContent).toContain('42 ms');
    expect(badge.element().textContent).not.toContain('stale bad key');
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
    installFetch((url) => {
      if (url.includes('/api/llm/models')) return json({ models: [] });
      if (url.includes('/api/llm/discover')) {
        return json({
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
        });
      }
      return json(baseData);
    });

    const screen = await mount();
    await screen.getByRole('button', { name: /scan local servers/i }).click();

    const card = screen.getByTestId('discovered-server-ollama');
    await expect.element(card).toBeVisible();
    expect(card.element().textContent).toContain('qwen2.5:14b');
    expect(card.element().textContent).toContain('num_ctx=4096');

    await card.getByRole('button', { name: /attach/i }).click();
    await expect.element(screen.getByLabelText('Ollama default model')).toHaveValue('qwen2.5:14b');
    await expect
      .poll(() => screen.getByLabelText('Toggle Ollama').element().textContent)
      .toBe('ENABLED');
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

    installFetch((url, init) => {
      if (url.includes('/api/llm/models')) {
        if (url.includes('provider=nvidia')) {
          return json({ models: [{ id: 'nvidia/unpriced-model', provider: 'nvidia', priced: false }] });
        }
        return json({ models: [{ id: 'local-qwen', provider: 'omlx' }] });
      }
      if (url.includes('/api/llm/settings') && init?.method === 'POST') {
        savedPayloads.push(JSON.parse(String(init.body)));
        return json({ ok: true });
      }
      if (url.includes('/api/llm/test') && init?.method === 'POST') {
        testPayloads.push(JSON.parse(String(init.body)));
        return json({
          success: true,
          provider: 'nvidia',
          wire: 'openai-chat',
          model: 'nvidia/bench-override',
          reply: 'pong',
          duration_ms: 19,
          attempt_count: 2,
          cost_usd: 0.00012,
        });
      }
      return json(data);
    });

    const screen = await mount();

    // Mono-model tribunal advisory + unpriced non-local warning
    await expect
      .element(screen.getByTestId('mono-model-tribunal-warning'))
      .toMatchTextContent('Ansatz Scout and Disconfirmation Officer share nvidia:nvidia/unpriced-model');
    await expect.poll(() => bodyText(screen)).toContain('Unpriced · loops blocked');

    // Differentiate disconfirmation-officer to clear warning
    await screen.getByLabelText('disconfirmation-officer provider').selectOptions('omlx');
    await screen.getByLabelText('disconfirmation-officer model').fill('local-qwen');
    await expect.poll(() => screen.container.querySelectorAll('[data-testid="mono-model-tribunal-warning"]').length).toBe(0);

    // Edit BaseURL and Wire for NVIDIA NIM
    await screen.getByLabelText('NVIDIA NIM base URL').fill('http://127.0.0.1:9001/v1');
    await screen.getByLabelText('NVIDIA NIM wire protocol').selectOptions('openai-responses');

    // Open copilot role options, set per-role temperature/seed/reasoning_effort and add fallback
    await screen.getByLabelText('Configure copilot sampling and fallbacks').click();
    await expect.element(screen.getByLabelText('copilot temperature')).toBeVisible();
    await screen.getByLabelText('copilot temperature').fill('0.15');
    await screen.getByLabelText('copilot seed').fill('7');
    await screen.getByLabelText('copilot reasoning effort').selectOptions('high');
    await screen.getByRole('button', { name: /add fallback/i }).click();

    await screen.getByRole('button', { name: /save settings/i }).click();
    await expect.poll(() => savedPayloads.length).toBe(1);

    const saved = savedPayloads[0] as {
      providers: Record<string, { base_url: string; wire?: string }>;
      role_assignments: Record<
        string,
        { temperature?: number; seed?: number; reasoning_effort?: string; fallback?: Array<{ provider: string; model: string }> }
      >;
    };
    expect(saved.providers.nvidia.base_url).toBe('http://127.0.0.1:9001/v1');
    expect(saved.providers.nvidia.wire).toBe('openai-responses');
    expect(saved.role_assignments.copilot.temperature).toBe(0.15);
    expect(saved.role_assignments.copilot.seed).toBe(7);
    expect(saved.role_assignments.copilot.reasoning_effort).toBe('high');
    expect(saved.role_assignments.copilot.fallback?.length).toBe(1);

    // Explicit benchmark provider+model override
    await screen.getByLabelText('Benchmark provider override').selectOptions('nvidia');
    await screen.getByLabelText('Benchmark model override').fill('nvidia/bench-override');
    await screen.getByRole('button', { name: /test model/i }).click();
    await expect.poll(() => testPayloads.length).toBe(1);
    expect(testPayloads[0]).toMatchObject({ provider: 'nvidia', model: 'nvidia/bench-override' });
    await expect.poll(() => bodyText(screen)).toContain('openai-chat');
    expect(bodyText(screen)).toContain('$0.00012');

    // Fill unassigned roles + Sovereign Local profile
    await screen.getByRole('button', { name: /fill unassigned roles/i }).click();
    await expect.element(screen.getByLabelText('epistemic-scribe model')).toHaveValue('nvidia/fleet-model');

    await screen.getByRole('button', { name: 'Sovereign Local' }).click();
    await expect.element(screen.getByLabelText('copilot provider')).toHaveValue('omlx');
    await expect.element(screen.getByLabelText('copilot model')).toHaveValue('local-qwen');
  });

  it('asks for explicit acknowledgement when the eval gate refuses a model change', async () => {
    const saves: Array<Record<string, unknown>> = [];
    let settingsWrites = 0;
    installFetch((url, init) => {
      if (url.includes('/api/llm/models')) return json({ models: [] });
      if (url.includes('/api/llm/settings') && init?.method === 'POST') {
        settingsWrites += 1;
        saves.push(JSON.parse(String(init.body)) as Record<string, unknown>);
        if (settingsWrites === 1) {
          return refusal(
            409,
            JSON.stringify({
              error: 'model_unevaluated',
              message: 'copilot model nvidia/new-model is unevaluated on the pinned fixtures',
            }),
          );
        }
        return json({ status: 'updated', settings: settings() });
      }
      return json(settings());
    });

    const screen = await mount();
    const save = screen.getByRole('button', { name: /save settings/i });
    await expect.element(save).toBeEnabled();
    await save.click();
    await expect.poll(() => settingsWrites).toBe(1);
    expect(saves[0].acknowledge_unevaluated).toBeUndefined();

    const ack = screen.getByTestId('llm-eval-ack');
    await expect.element(ack).toBeVisible();
    await ack.click();
    await expect.poll(() => settingsWrites).toBe(2);
    expect(saves[1].acknowledge_unevaluated).toBe(true);
  });
});
