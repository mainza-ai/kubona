import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, type RenderResult } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import { CommandDock } from './CommandDock';
import { MemoryPanel } from '../MemoryPanel';
import { publishInvalidation } from '@/domain/invalidation';
import { setAuthPhase } from '@/domain/auth-state';

type LlmSettings = Record<string, unknown>;

const INITIAL_SETTINGS: LlmSettings = {
  air_gapped: false,
  active_provider: 'omlx',
  active_model: 'model-a',
  role_assignments: {
    copilot: { role: 'copilot', provider: 'omlx', model: 'model-a' },
    'epistemic-scribe': { role: 'epistemic-scribe', provider: 'omlx', model: 'model-a' },
    'ansatz-scout': { role: 'ansatz-scout', provider: 'omlx', model: '' },
    coder: { role: 'coder', provider: 'anthropic', model: 'claude-3-7-sonnet' },
  },
};

const CONTEXT = {
  session_id: 's1',
  turns: 1,
  turn_cap: 40,
  history_chars: 120,
  history_chars_cap: 48000,
  would_drop: 0,
  pressure_pct: 10,
  prompt_tokens: 5,
  completion_tokens: 2,
};

function json(body: unknown) {
  return { ok: true, status: 200, json: async () => body };
}

// The dock hydrates the active model label on mount and again when the model
// morph opens, so every endpoint it touches is answered at the transport by
// URL + method instead of mocking '@/lib/api'.
function installFetch(options: { settings?: () => LlmSettings } = {}) {
  const calls: Array<{ method: string; path: string; body?: unknown }> = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({ method, path, body: init?.body });
    switch (path) {
      case '/api/llm/settings':
        if (method === 'POST') return json({ status: 'saved', changed: true, settings: {} });
        return json(options.settings?.() ?? INITIAL_SETTINGS);
      case '/api/llm/models':
        return json({ models: [{ id: 'model-a' }, { id: 'model-b' }] });
      // No recorded decision evaluation for these fixtures: the badge stays
      // "unevaluated" and the picker rows under test are the real DOM rows.
      case '/api/evals/llm':
        return { ok: false, status: 404, text: async () => '', json: async () => ({}) };
      case '/api/sessions/context':
        return json(CONTEXT);
      case '/api/memory/recall':
        return json({ snippets: [], tenant: 'default' });
      case '/api/memory/retain':
        return json({ id: 'mem-1234567890abcd', bank: 'experiences', importance: 0.5 });
      default:
        throw new Error(`unexpected fetch ${method} ${path}`);
    }
  });
  return calls;
}

function savedSettingsBodies(calls: Array<{ method: string; path: string; body?: unknown }>) {
  return calls
    .filter((call) => call.method === 'POST' && call.path === '/api/llm/settings')
    .map((call) => JSON.parse(String(call.body)) as Record<string, unknown>);
}

/** Click the model row for `id` once the picker is idle (saving disables rows). */
async function pickModelRow(screen: RenderResult, id: string) {
  const row = () => screen.container.querySelector(`[data-model-id="${id}"]`) as HTMLButtonElement | null;
  await expect.poll(() => row() !== null).toBe(true);
  await expect.poll(() => row()?.disabled === false).toBe(true);
  await userEvent.click(row()!);
}

beforeEach(() => {
  // Writes require an authenticated phase (no operator bearer in tests).
  setAuthPhase('authenticated');
  // Model pins/recent live in localStorage; a clean document per test keeps
  // one case from pre-populating the next.
  window.localStorage.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('CommandDock in a real browser', () => {
  it('toggles Quick Actions morph and convenes Tribunal with custom topic', async () => {
    installFetch();
    const onTribunal = vi.fn(async () => {});
    const onFollowUp = vi.fn();
    const screen = await render(
      <CommandDock
        onSubmit={vi.fn()}
        onTribunal={onTribunal}
        onFollowUp={onFollowUp}
        followUps={[{ label: 'H₂ Bond PES', tone: 'warning', key: 'send-pes' }]}
        targetDisplayName="Hydrogen Molecule (H₂)"
      />,
    );

    const toggle = screen.getByRole('button', { name: 'Quick actions and tribunal', exact: true });
    await expect.element(toggle).toBeVisible();
    await toggle.click();

    const region = screen.getByRole('region', { name: 'Quick actions and tribunal', exact: true });
    await expect.element(region).toBeVisible();
    await expect.element(screen.getByRole('button', { name: /H₂ Bond PES/ })).toBeVisible();

    const topic = screen.getByLabelText('Tribunal deliberation topic');
    await topic.fill('Spin-orbit coupling');
    await screen.getByRole('button', { name: /convene multi-agent research tribunal/i }).click();

    await expect.poll(() => onTribunal.mock.calls.length).toBe(1);
    expect(onTribunal).toHaveBeenCalledWith('Spin-orbit coupling');
    // The morph closes once the tribunal is convened.
    await expect.poll(() => toggle.element().getAttribute('aria-expanded')).toBe('false');
  });

  it('does not show a dead clear control for campaign context', async () => {
    installFetch();
    const onClearContext = vi.fn();
    const screen = await render(
      <CommandDock onSubmit={vi.fn()} contextLabel="Lithium Hydride (LiH)" />,
    );

    await expect.element(screen.getByText('Lithium Hydride (LiH)')).toBeVisible();
    expect(screen.container.querySelector('[aria-label="Clear context"]')).toBeNull();

    // Complement: the control exists exactly when the handler does, so the
    // negative assertion above is meaningful rather than vacuous.
    await screen.rerender(
      <CommandDock
        onSubmit={vi.fn()}
        contextLabel="Lithium Hydride (LiH)"
        onClearContext={onClearContext}
      />,
    );
    const clear = screen.getByRole('button', { name: 'Clear context', exact: true });
    await expect.element(clear).toBeVisible();
    await clear.click();
    expect(onClearContext).toHaveBeenCalledTimes(1);
  });

  it('displays effective copilot model on mount before Model morph is opened', async () => {
    installFetch({
      settings: () => ({
        air_gapped: false,
        active_provider: 'omlx',
        active_model: 'fallback-model',
        role_assignments: {
          copilot: { role: 'copilot', provider: 'nvidia', model: 'nvidia/copilot-special' },
        },
      }),
    });
    const screen = await render(<CommandDock onSubmit={vi.fn()} />);

    const hubText = () =>
      screen.container.querySelector('[aria-label="Open Model Hub"]')?.textContent ?? '';
    await expect.element(screen.getByRole('button', { name: 'Open Model Hub', exact: true })).toBeVisible();
    await expect.poll(hubText).toContain('nvidia/copilot-special');
    // The morph was never opened: the label came from the mount hydration.
    expect(screen.container.querySelector('[aria-label="Quick model switcher"]')).toBeNull();
  });

  it('opens inline model morph on Open Model Hub without invoking onOpenModel until Open LLM Settings is clicked', async () => {
    installFetch();
    const onOpenModel = vi.fn();
    const screen = await render(<CommandDock onSubmit={vi.fn()} onOpenModel={onOpenModel} />);

    await screen.getByRole('button', { name: 'Open Model Hub', exact: true }).click();
    await expect
      .element(screen.getByRole('region', { name: 'Quick model switcher', exact: true }))
      .toBeVisible();
    expect(onOpenModel).not.toHaveBeenCalled();

    await screen.getByRole('button', { name: /open llm settings/i }).click();
    await expect.poll(() => onOpenModel.mock.calls.length).toBe(1);
  });

  it('switches copilot and active model inline from Model morph without touching unrelated roles unless sync toggle is active', async () => {
    const calls = installFetch();
    const screen = await render(<CommandDock onSubmit={vi.fn()} onOpenModel={vi.fn()} />);

    await screen.getByRole('button', { name: 'Quick switch model', exact: true }).click();
    await expect
      .element(screen.getByRole('region', { name: 'Quick model switcher', exact: true }))
      .toBeVisible();
    await pickModelRow(screen, 'model-b');

    await expect.poll(() => savedSettingsBodies(calls).length).toBe(1);
    const first = savedSettingsBodies(calls)[0];
    expect(first).toMatchObject({
      active_provider: 'omlx',
      active_model: 'model-b',
      role_assignments: {
        copilot: { role: 'copilot', provider: 'omlx', model: 'model-b' },
        'epistemic-scribe': { role: 'epistemic-scribe', provider: 'omlx', model: 'model-a' },
        coder: { role: 'coder', provider: 'anthropic', model: 'claude-3-7-sonnet' },
      },
    });

    // Toggle sync mode to "Copilot + default roles" and re-switch: matching
    // defaults and unassigned roles follow, the customized coder stays intact.
    await screen.getByRole('button', { name: /copilot only/i }).click();
    await expect
      .element(screen.getByRole('button', { name: /copilot \+ default roles/i }))
      .toBeVisible();

    await pickModelRow(screen, 'model-b');
    await expect.poll(() => savedSettingsBodies(calls).length).toBe(2);
    const second = savedSettingsBodies(calls)[1];
    expect(second).toMatchObject({
      active_provider: 'omlx',
      active_model: 'model-b',
      role_assignments: {
        copilot: { role: 'copilot', provider: 'omlx', model: 'model-b' },
        'epistemic-scribe': { role: 'epistemic-scribe', provider: 'omlx', model: 'model-b' },
        'ansatz-scout': { role: 'ansatz-scout', provider: 'omlx', model: 'model-b' },
        coder: { role: 'coder', provider: 'anthropic', model: 'claude-3-7-sonnet' },
      },
    });
  });

  it('renders Stop generation while streaming instead of Send', async () => {
    installFetch();
    const onStop = vi.fn();
    const screen = await render(<CommandDock onSubmit={vi.fn()} streaming onStop={onStop} />);

    const stop = screen.getByRole('button', { name: 'Stop generation', exact: true });
    await expect.element(stop).toBeVisible();
    await stop.click();
    await expect.poll(() => onStop.mock.calls.length).toBe(1);
    expect(screen.container.querySelector('[aria-label="Send to copilot"]')).toBeNull();
  });

  it('renders an inline Inspector trigger on the dock row when onOpenInspector is provided', async () => {
    installFetch();
    const onOpenInspector = vi.fn();
    const screen = await render(
      <CommandDock onSubmit={vi.fn()} onOpenInspector={onOpenInspector} inspectorShortcut="⌘I" />,
    );

    const trigger = screen.container.querySelector(
      '[title="Open Inspector (⌘I)"]',
    ) as HTMLButtonElement | null;
    expect(trigger).not.toBeNull();
    expect(trigger!.textContent).toContain('Inspector');
    expect(trigger!.textContent).toContain('⌘I');
    await userEvent.click(trigger!);
    await expect.poll(() => onOpenInspector.mock.calls.length).toBe(1);
  });

  it('tracks Remembered counter in CommandDock and fails closed on append-only MemoryPanel undo unless onRetract is supplied', async () => {
    installFetch();
    const screen = await render(<CommandDock onSubmit={vi.fn()} sessionId="s1" />);

    const memToggle = () => screen.getByRole('button', { name: 'Session memory', exact: true });
    await expect.element(memToggle()).toBeVisible();
    expect(memToggle().element().textContent).toContain('Memory');
    await memToggle().click();

    const note = screen.getByLabelText('Operator note to retain');
    await expect.element(note).toBeVisible();
    await note.fill('Check UCCSD depth');
    await screen.getByRole('button', { name: 'Retain operator note', exact: true }).click();

    await expect.poll(() => memToggle().element().textContent).toContain('Remembered (1)');
    const receipt = screen.container.querySelector('[data-testid="remembered-receipt"]');
    expect(receipt).not.toBeNull();
    const appendOnlyUndo = receipt!.querySelector('button') as HTMLButtonElement;
    expect(appendOnlyUndo.disabled).toBe(true);
    expect(appendOnlyUndo.textContent).toContain('Undo (ledger append-only)');
    const restore = screen.container.querySelector(
      '[title="Restore last retained text into the draft field to supersede or revise"]',
    );
    expect(restore!.textContent).toContain('Restore draft');

    // When onRetract IS supplied, Undo invokes real retraction. The dock is
    // unmounted first so exactly one memory panel exists in the document.
    await screen.unmount();
    const onRetract = vi.fn(async () => ({ ok: true }));
    const panel = await render(<MemoryPanel sessionId="s1" defaultOpen onRetract={onRetract} />);
    await panel.getByLabelText('Operator note to retain').fill('Retractable note');
    await panel.getByRole('button', { name: 'Retain operator note', exact: true }).click();

    const panelUndo = () =>
      panel.container.querySelector('[data-testid="remembered-receipt"] button') as HTMLButtonElement | null;
    await expect.poll(() => panelUndo()?.disabled === false).toBe(true);
    expect(panelUndo()!.textContent).toContain('Undo');
    await userEvent.click(panelUndo()!);
    await expect.poll(() => onRetract.mock.calls.length).toBe(1);
    expect(onRetract).toHaveBeenCalledWith('mem-1234567890abcd');
  });

  it('refreshes active copilot model label when an llm domain invalidation arrives', async () => {
    let current: LlmSettings = INITIAL_SETTINGS;
    installFetch({ settings: () => current });
    const screen = await render(<CommandDock onSubmit={vi.fn()} />);

    const hubText = () =>
      screen.container.querySelector('[aria-label="Open Model Hub"]')?.textContent ?? '';
    await expect.poll(hubText).toContain('model-a');

    current = {
      air_gapped: false,
      active_provider: 'openai',
      active_model: 'gpt-4.1',
      role_assignments: {
        copilot: { role: 'copilot', provider: 'openai', model: 'gpt-4.1' },
      },
    };
    publishInvalidation(new Set(['llm']));

    await expect.poll(hubText).toContain('gpt-4.1');
  });
});
