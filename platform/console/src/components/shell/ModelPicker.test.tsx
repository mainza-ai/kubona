import { describe, expect, it, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { ModelPicker, type PickerProviderConfig, type PickerModelItem } from './ModelPicker';

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map((item) => textOf(item)).join(' ');
  if (node && typeof node === 'object') {
    const kids = (node as { children?: unknown[] }).children ?? [];
    return kids.map((c) => textOf(c)).join(' ');
  }
  return '';
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

describe('ModelPicker', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    if (typeof window !== 'undefined' && window.localStorage) {
      window.localStorage.clear();
    }
  });

  it('switches provider tabs, filters models, displays capability & pricing/unpriced badges, and pins favorites', async () => {
    const onSelectProviderTab = vi.fn();
    const onSelectModel = vi.fn(async () => {});
    let tree: TestRenderer.ReactTestRenderer;

    await act(async () => {
      tree = TestRenderer.create(
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
    });

    // Local tab shows context window + Tools + Reasoning + local $0 badges
    const qwenBtn = tree!.root.findByProps({ 'data-model-id': 'qwen-32b' });
    expect(textOf(qwenBtn)).toContain('Tools');
    expect(textOf(qwenBtn)).toContain('Reasoning');
    expect(textOf(qwenBtn)).toContain('local $0');

    // Switch to OpenAI tab
    const openaiTab = tree!.root
      .findAllByProps({ role: 'tab' })
      .find((t) => textOf(t).includes('openai'))!;
    await act(async () => {
      openaiTab.props.onClick();
    });
    expect(onSelectProviderTab).toHaveBeenCalledWith('openai');

    // Unpriced non-local warning appears
    const unpricedBtn = tree!.root.findByProps({ 'data-model-id': 'unpriced-preview' });
    expect(textOf(unpricedBtn)).toContain('unpriced · loops blocked');

    // Favorite toggle persists a pinned row
    const favBtn = tree!.root.findByProps({ 'aria-label': 'Favorite gpt-4.1' });
    act(() => {
      favBtn.props.onClick();
    });
    expect(textOf(tree!.root)).toContain('Pinned & Recent');

    // Search filters down cleanly and selects model with sampling overrides
    const searchInput = tree!.root.findByProps({ 'aria-label': 'Search models' });
    act(() => {
      searchInput.props.onChange({ target: { value: 'gpt-4.1' } });
    });
    expect(tree!.root.findAllByProps({ 'data-model-id': 'unpriced-preview' })).toHaveLength(0);

    const gptBtns = tree!.root.findAllByProps({ 'data-model-id': 'gpt-4.1' });
    expect(gptBtns).toHaveLength(2);
    await act(async () => {
      gptBtns[0].props.onClick();
    });
    expect(onSelectModel).toHaveBeenCalledWith('openai', 'gpt-4.1', {});
  });

  it('supports inline Copilot sampling save and blocks cloud picks when air-gapped', async () => {
    const onApplySampling = vi.fn(async () => {});
    const onClose = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;

    await act(async () => {
      tree = TestRenderer.create(
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
    });

    expect(textOf(tree!.root)).toContain('blocked while Sovereign Air-Gap is active');
    expect(tree!.root.findByProps({ 'data-model-id': 'gpt-4.1' }).props.disabled).toBe(true);

    // Open sampling controls & save temperature/seed/reasoning_effort
    const samplingBtn = tree!.root
      .findAllByType('button')
      .find((b) => textOf(b).includes('Sampling'))!;
    act(() => {
      samplingBtn.props.onClick();
    });
    act(() => {
      tree!.root.findByProps({ 'aria-label': 'Copilot reasoning effort' }).props.onChange({ target: { value: 'high' } });
      tree!.root.findByProps({ 'aria-label': 'Copilot temperature override' }).props.onChange({ target: { value: '0.2' } });
      tree!.root.findByProps({ 'aria-label': 'Copilot deterministic seed' }).props.onChange({ target: { value: '42' } });
    });
    const saveSamplingBtn = tree!.root
      .findAllByType('button')
      .find((b) => textOf(b).includes('Save Copilot sampling'))!;
    await act(async () => {
      saveSamplingBtn.props.onClick();
    });
    expect(onApplySampling).toHaveBeenCalledWith({
      temperature: 0.2,
      seed: 42,
      reasoning_effort: 'high',
    });

    // Escape closes cleanly
    const region = tree!.root.findByProps({ 'aria-label': 'Quick model switcher' });
    act(() => {
      region.props.onKeyDown({ key: 'Escape', preventDefault: vi.fn() });
    });
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
