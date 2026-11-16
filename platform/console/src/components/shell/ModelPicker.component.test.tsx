import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import { ModelPicker, type PickerProviderConfig, type PickerModelItem } from './ModelPicker';

// The picker probes the decision-eval rail on mount; answer it at the
// transport so the badge stays "unevaluated" and the rows under test are the
// real DOM rows the operator sees.
function stubEvalStatus() {
  const fetchStub = vi.fn(async (url: string | URL | Request) => {
    const path = new URL(String(url)).pathname;
    if (path !== '/api/evals/llm') throw new Error(`unexpected fetch ${path}`);
    return {
      ok: true,
      status: 200,
      json: async () => ({
        role: 'copilot',
        provider: 'omlx',
        model: 'qwen-32b',
        status: 'unevaluated',
        evaluated: 0,
        total: 0,
        unscored: 0,
        evaluated_at: null,
        fixtures_sha256: 'test-fixtures',
        brier: null,
        ece: null,
        labels: [],
      }),
    };
  });
  vi.stubGlobal('fetch', fetchStub);
  return fetchStub;
}

const PROVIDERS: Record<string, PickerProviderConfig> = {
  omlx: { type: 'omlx', name: 'oMLX', enabled: true, is_local: true },
  openai: {
    type: 'openai',
    name: 'OpenAI',
    enabled: true,
    is_local: false,
    has_key: true,
    key_status: 'valid',
  },
  anthropic: {
    type: 'anthropic',
    name: 'Anthropic',
    enabled: false,
    is_local: false,
    has_key: false,
    key_status: 'missing',
  },
};

const MODELS: Record<string, PickerModelItem[]> = {
  omlx: [
    {
      id: 'qwen-32b',
      context_window: 131072,
      capabilities: { tools: true, reasoning: true },
    },
  ],
  openai: [
    {
      id: 'gpt-4.1',
      context_window: 1048576,
      capabilities: { tools: true, vision: true, structured_output: true },
      priced: true,
      price_source: 'rate-card',
      input_per_1k_usd: 0.002,
      output_per_1k_usd: 0.008,
    },
    {
      id: 'unpriced-preview',
      context_window: 32768,
      priced: false,
    },
  ],
};

beforeEach(() => {
  // Pins live in localStorage; a clean document per test keeps one case from
  // pre-populating the next.
  window.localStorage.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('ModelPicker in a real browser', () => {
  it('switches provider tabs, filters models, displays capability & pricing/unpriced badges, and pins favorites', async () => {
    stubEvalStatus();
    const onSelectProviderTab = vi.fn();
    const onSelectModel = vi.fn(async () => {});

    const screen = await render(
      <ModelPicker
        activeProvider="omlx"
        activeModel="qwen-32b"
        providers={PROVIDERS}
        modelsByProvider={MODELS}
        syncMatchingRoles={false}
        onToggleSyncMatchingRoles={vi.fn()}
        onSelectProviderTab={onSelectProviderTab}
        onSelectModel={onSelectModel}
        onClose={vi.fn()}
      />,
    );
    const container = screen.container;

    // Local tab shows context window + Tools + Reasoning + local $0 badges.
    const qwenRow = container.querySelector('[data-model-id="qwen-32b"]') as HTMLElement;
    expect(qwenRow).not.toBeNull();
    expect(qwenRow.textContent).toContain('Tools');
    expect(qwenRow.textContent).toContain('Reasoning');
    expect(qwenRow.textContent).toContain('local $0');

    // Switch to the OpenAI tab with a real click.
    await screen.getByRole('tab', { name: /openai/i }).click();
    expect(onSelectProviderTab).toHaveBeenCalledWith('openai');

    // Unpriced non-local warning appears on the cloud tab.
    await expect
      .poll(() => container.querySelector('[data-model-id="unpriced-preview"]')?.textContent ?? '')
      .toContain('unpriced · loops blocked');

    // Favorite toggle persists a pinned row.
    await screen.getByRole('button', { name: 'Favorite gpt-4.1' }).click();
    await expect.element(screen.getByText(/Pinned & Recent/)).toBeVisible();

    // Search filters down cleanly and selects the model with sampling overrides.
    await screen.getByLabelText('Search models').fill('gpt-4.1');
    await expect.poll(() => container.querySelectorAll('[data-model-id="unpriced-preview"]').length).toBe(0);
    // Two rows: the pinned favorite plus the catalog entry.
    await expect.poll(() => container.querySelectorAll('[data-model-id="gpt-4.1"]').length).toBe(2);

    const gptRows = container.querySelectorAll('[data-model-id="gpt-4.1"]');
    await userEvent.click(gptRows[0] as HTMLElement);
    await expect.poll(() => onSelectModel.mock.calls.length).toBe(1);
    expect(onSelectModel).toHaveBeenCalledWith('openai', 'gpt-4.1', {});
  });

  it('supports inline Copilot sampling save and blocks cloud picks when air-gapped', async () => {
    stubEvalStatus();
    const onApplySampling = vi.fn(async () => {});
    const onClose = vi.fn();

    const screen = await render(
      <ModelPicker
        activeProvider="openai"
        activeModel="gpt-4.1"
        airGapped
        providers={PROVIDERS}
        modelsByProvider={MODELS}
        syncMatchingRoles={false}
        onToggleSyncMatchingRoles={vi.fn()}
        onSelectProviderTab={vi.fn()}
        onSelectModel={vi.fn()}
        onApplySampling={onApplySampling}
        onClose={onClose}
      />,
    );

    await expect.element(screen.getByText(/blocked while Sovereign Air-Gap is active/)).toBeVisible();
    const cloudRow = screen.container.querySelector('[data-model-id="gpt-4.1"]') as HTMLButtonElement;
    expect(cloudRow.disabled).toBe(true);

    // Open sampling controls & save temperature/seed/reasoning_effort.
    await screen.getByRole('button', { name: /Sampling/ }).click();
    await screen.getByLabelText('Copilot reasoning effort').selectOptions('high');
    await screen.getByLabelText('Copilot temperature override').fill('0.2');
    await screen.getByLabelText('Copilot deterministic seed').fill('42');
    await screen.getByRole('button', { name: 'Save Copilot sampling' }).click();
    await expect.poll(() => onApplySampling.mock.calls.length).toBe(1);
    expect(onApplySampling).toHaveBeenCalledWith({
      temperature: 0.2,
      seed: 42,
      reasoning_effort: 'high',
    });

    // Escape from the focused search field closes the switcher cleanly.
    const search = screen.getByLabelText('Search models');
    await search.click();
    await expect.element(search).toHaveFocus();
    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('explains the auth gate instead of attempting a write when anonymous', async () => {
    stubEvalStatus();
    const onRequestSignIn = vi.fn();

    const screen = await render(
      <ModelPicker
        activeProvider="omlx"
        activeModel="qwen-32b"
        providers={PROVIDERS}
        modelsByProvider={MODELS}
        syncMatchingRoles={false}
        writable={false}
        onRequestSignIn={onRequestSignIn}
        onToggleSyncMatchingRoles={vi.fn()}
        onSelectProviderTab={vi.fn()}
        onSelectModel={vi.fn(async () => {})}
        onClose={vi.fn()}
      />,
    );

    await expect.element(screen.getByText(/Sign in to change the copilot model/)).toBeVisible();
    await screen.getByRole('button', { name: 'Open sign-in' }).click();
    expect(onRequestSignIn).toHaveBeenCalledTimes(1);
  });
});
