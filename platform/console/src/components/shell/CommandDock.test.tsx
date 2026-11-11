import { describe, expect, it, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import * as api from '@/lib/api';
import { publishInvalidation } from '@/domain/invalidation';
import { CommandDock } from './CommandDock';
import { MemoryPanel } from '../MemoryPanel';

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map((item) => textOf(item)).join(' ');
  if (node && typeof node === 'object') {
    const kids = (node as { children?: unknown[] }).children ?? [];
    return kids.map((c) => textOf(c)).join(' ');
  }
  return '';
}

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>();
  return {
    ...actual,
    fetchLLMSettings: vi.fn(async () => ({
      active_provider: 'omlx',
      active_model: 'model-a',
      role_assignments: {
        copilot: { role: 'copilot', provider: 'omlx', model: 'model-a' },
        'epistemic-scribe': { role: 'epistemic-scribe', provider: 'omlx', model: 'model-a' },
        'ansatz-scout': { role: 'ansatz-scout', provider: 'omlx', model: '' },
        coder: { role: 'coder', provider: 'anthropic', model: 'claude-3-7-sonnet' },
      },
    })),
    fetchLLMModels: vi.fn(async () => ({ models: [{ id: 'model-a' }, { id: 'model-b' }] })),
    saveLLMSettings: vi.fn(async () => ({ ok: true })),
    fetchContextStatus: vi.fn(async () => null),
    fetchSessionOverflow: vi.fn(async () => null),
    fetchMemoryRecall: vi.fn(async () => ({ snippets: [] })),
    retainMemory: vi.fn(async (_sid: string, _content: string, opts?: { bank?: string; importance?: number }) => ({
      id: 'mem-1234567890abcd',
      bank: opts?.bank ?? 'experiences',
      importance: opts?.importance ?? 0.5,
    })),
  };
});

describe('CommandDock', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    if (typeof window !== 'undefined' && window.localStorage) {
      window.localStorage.clear();
    }
  });

  it('toggles Quick Actions morph and convenes Tribunal with custom topic', async () => {
    const onTribunal = vi.fn(async () => {});
    const onFollowUp = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <CommandDock
          onSubmit={vi.fn()}
          onTribunal={onTribunal}
          onFollowUp={onFollowUp}
          followUps={[{ label: 'H₂ Bond PES', tone: 'warning', key: 'send-pes' }]}
          targetDisplayName="Hydrogen Molecule (H₂)"
        />,
      );
    });

    const toggle = tree!.root.findByProps({ 'aria-label': 'Quick actions and tribunal' });
    await act(async () => {
      toggle.props.onClick();
    });

    const topicInput = tree!.root.findByProps({ 'aria-label': 'Tribunal deliberation topic' });
    act(() => {
      topicInput.props.onChange({ target: { value: 'Spin-orbit coupling' } });
    });

    const convene = tree!.root.findAllByType('button').find((b) =>
      textOf(b).includes('Convene Multi-Agent Research Tribunal'),
    )!;
    await act(async () => {
      convene.props.onClick();
    });

    expect(onTribunal).toHaveBeenCalledWith('Spin-orbit coupling');
  });

  it('displays effective copilot model on mount before Model morph is opened', async () => {
    vi.mocked(api.fetchLLMSettings).mockResolvedValueOnce({
      active_provider: 'omlx',
      active_model: 'fallback-model',
      role_assignments: {
        copilot: { role: 'copilot', provider: 'nvidia', model: 'nvidia/copilot-special' },
      },
    } as never);
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<CommandDock onSubmit={vi.fn()} />);
    });
    const hubBtn = tree!.root.findByProps({ 'aria-label': 'Open Model Hub' });
    expect(textOf(hubBtn)).toContain('nvidia/copilot-special');
  });

  it('opens inline model morph on Open Model Hub without invoking onOpenModel until Open LLM Settings is clicked', async () => {
    const onOpenModel = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<CommandDock onSubmit={vi.fn()} onOpenModel={onOpenModel} />);
    });
    const hubBtn = tree!.root.findByProps({ 'aria-label': 'Open Model Hub' });
    await act(async () => {
      hubBtn.props.onClick();
    });
    await act(async () => {
      await Promise.resolve();
    });
    expect(onOpenModel).not.toHaveBeenCalled();
    expect(tree!.root.findByProps({ 'aria-label': 'Quick model switcher' })).toBeDefined();

    const settingsBtn = tree!.root.findAllByType('button').find((b) => textOf(b).includes('Open LLM Settings →'))!;
    act(() => {
      settingsBtn.props.onClick();
    });
    expect(onOpenModel).toHaveBeenCalledTimes(1);
  });

  it('switches copilot and active model inline from Model morph without touching unrelated roles unless sync toggle is active', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<CommandDock onSubmit={vi.fn()} onOpenModel={vi.fn()} />);
    });

    const quickModel = tree!.root.findByProps({ 'aria-label': 'Quick switch model' });
    await act(async () => {
      quickModel.props.onClick();
    });
    await act(async () => {
      await Promise.resolve();
    });

    const modelBtn = tree!.root.findAllByType('button').find((b) => b.props['data-model-id'] === 'model-b')!;
    await act(async () => {
      modelBtn.props.onClick();
    });

    expect(api.saveLLMSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({
        active_provider: 'omlx',
        active_model: 'model-b',
        role_assignments: expect.objectContaining({
          copilot: { role: 'copilot', provider: 'omlx', model: 'model-b' },
          'epistemic-scribe': { role: 'epistemic-scribe', provider: 'omlx', model: 'model-a' },
          coder: { role: 'coder', provider: 'anthropic', model: 'claude-3-7-sonnet' },
        }),
      }),
    );

    // Toggle sync mode to "Copilot + default roles" and re-switch: matching defaults + unassigned sync, customized coder stays intact.
    const syncToggle = tree!.root.findAllByType('button').find((b) => textOf(b).includes('Copilot only'))!;
    act(() => {
      syncToggle.props.onClick();
    });
    expect(textOf(syncToggle)).toContain('Copilot + default roles');

    const modelBtn2 = tree!.root.findAllByType('button').find((b) => b.props['data-model-id'] === 'model-b')!;
    await act(async () => {
      modelBtn2.props.onClick();
    });
    expect(api.saveLLMSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({
        role_assignments: expect.objectContaining({
          copilot: { role: 'copilot', provider: 'omlx', model: 'model-b' },
          'epistemic-scribe': { role: 'epistemic-scribe', provider: 'omlx', model: 'model-b' },
          'ansatz-scout': { role: 'ansatz-scout', provider: 'omlx', model: 'model-b' },
          coder: { role: 'coder', provider: 'anthropic', model: 'claude-3-7-sonnet' },
        }),
      }),
    );
  });

  it('renders Stop generation while streaming instead of Send', () => {
    const onStop = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<CommandDock onSubmit={vi.fn()} streaming onStop={onStop} />);
    });
    const stopBtn = tree!.root.findByProps({ 'aria-label': 'Stop generation' });
    act(() => {
      stopBtn.props.onClick();
    });
    expect(onStop).toHaveBeenCalledTimes(1);
    expect(tree!.root.findAllByProps({ 'aria-label': 'Send to copilot' })).toHaveLength(0);
  });

  it('renders an inline Inspector trigger on the dock row when onOpenInspector is provided', () => {
    const onOpenInspector = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <CommandDock onSubmit={vi.fn()} onOpenInspector={onOpenInspector} inspectorShortcut="⌘I" />,
      );
    });
    const btn = tree!.root.findByProps({ title: 'Open Inspector (⌘I)' });
    expect(textOf(btn)).toContain('Inspector');
    expect(textOf(btn)).toContain('⌘I');
    act(() => {
      btn.props.onClick();
    });
    expect(onOpenInspector).toHaveBeenCalledTimes(1);
  });

  it('tracks Remembered counter in CommandDock and fails closed on append-only MemoryPanel undo unless onRetract is supplied', async () => {
    let dockTree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      dockTree = TestRenderer.create(<CommandDock onSubmit={vi.fn()} sessionId="s1" />);
    });
    const memToggle = () => dockTree!.root.findByProps({ 'aria-label': 'Session memory' });
    expect(textOf(memToggle())).toContain('Memory');
    await act(async () => { memToggle().props.onClick(); });

    const noteInput = dockTree!.root.findByProps({ 'aria-label': 'Operator note to retain' });
    act(() => { noteInput.props.onChange({ target: { value: 'Check UCCSD depth' } }); });
    const retainBtn = dockTree!.root.findByProps({ 'aria-label': 'Retain operator note' });
    await act(async () => { retainBtn.props.onClick(); });
    expect(textOf(memToggle())).toContain('Remembered (1)');
    const receiptItem = dockTree!.root.findByProps({ 'data-testid': 'remembered-receipt' });
    const appendOnlyBtn = receiptItem.findByType('button');
    expect(appendOnlyBtn.props.disabled).toBe(true);
    expect(textOf(appendOnlyBtn)).toContain('Undo (ledger append-only)');
    expect(textOf(dockTree!.root.findByProps({ title: 'Restore last retained text into the draft field to supersede or revise' }))).toContain('Restore draft');

    // When onRetract IS supplied, Undo invokes real retraction.
    const onRetract = vi.fn(async () => ({ ok: true }));
    let panelTree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      panelTree = TestRenderer.create(<MemoryPanel sessionId="s1" defaultOpen onRetract={onRetract} />);
    });
    const pInput = panelTree!.root.findByProps({ 'aria-label': 'Operator note to retain' });
    act(() => { pInput.props.onChange({ target: { value: 'Retractable note' } }); });
    const pRetain = panelTree!.root.findByProps({ 'aria-label': 'Retain operator note' });
    await act(async () => { pRetain.props.onClick(); });
    const undoBtn = panelTree!.root.findByProps({ 'data-testid': 'remembered-receipt' }).findByType('button');
    expect(undoBtn.props.disabled).toBe(false);
    await act(async () => { undoBtn.props.onClick(); });
    expect(onRetract).toHaveBeenCalledWith('mem-1234567890abcd');
  });

  it('refreshes active copilot model label when an llm domain invalidation arrives', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<CommandDock onSubmit={vi.fn()} />);
    });
    expect(textOf(tree!.root.findByProps({ 'aria-label': 'Open Model Hub' }))).toContain('model-a');

    vi.mocked(api.fetchLLMSettings).mockResolvedValueOnce({
      active_provider: 'openai',
      active_model: 'gpt-4.1',
      role_assignments: {
        copilot: { role: 'copilot', provider: 'openai', model: 'gpt-4.1' },
      },
    } as never);

    await act(async () => {
      publishInvalidation(new Set(['llm']));
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(textOf(tree!.root.findByProps({ 'aria-label': 'Open Model Hub' }))).toContain('gpt-4.1');
  });
});
