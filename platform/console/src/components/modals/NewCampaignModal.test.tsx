import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { NewCampaignModal } from './NewCampaignModal';

type CreateParams = {
  title: string;
  targetMolecule: string;
  spendLimitUSD: number;
  fresh: boolean;
};

async function renderModal(opts: {
  onCreate?: (p: CreateParams) => Promise<void>;
  onClose?: () => void;
} = {}): Promise<{ tree: TestRenderer.ReactTestRenderer; onCreate: (p: CreateParams) => Promise<void>; onClose: () => void }> {
  const onCreate = opts.onCreate ?? (async () => {});
  const onClose = opts.onClose ?? (() => {});
  let tree: TestRenderer.ReactTestRenderer;
  await act(async () => {
    tree = TestRenderer.create(<NewCampaignModal isOpen onClose={onClose} onCreate={onCreate} />);
  });
  return { tree: tree!, onCreate, onClose };
}

beforeEach(() => {
  // Node test env has no window; the modal's Escape-key effect needs it.
  vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn() });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

async function submit(tree: TestRenderer.ReactTestRenderer) {
  await act(async () => {
    await tree.root.findByType('form').props.onSubmit({ preventDefault: () => {} });
  });
}

function setInput(tree: TestRenderer.ReactTestRenderer, label: string, value: string) {
  act(() => {
    tree.root.findByProps({ 'aria-label': label }).props.onChange({ target: { value } });
  });
}

describe('NewCampaignModal inline validation (Wave-2)', () => {
  it('exposes the title and custom-molecule fields by label', async () => {
    const { tree } = await renderModal();
    expect(tree.root.findByProps({ 'aria-label': 'Research campaign name or hypothesis' }).type).toBe('input');
    expect(tree.root.findByProps({ 'aria-label': 'Custom chemical formula or SMILES' }).type).toBe('input');
    // Every label is associated with its input.
    for (const label of tree.root.findAllByType('label')) {
      if (label.props.className === 'sr-only') continue;
      const htmlFor = (label.props as { htmlFor?: string }).htmlFor;
      if (htmlFor) {
        expect(tree.root.findAll((n) => (n.props as { id?: string }).id === htmlFor).length).toBeGreaterThan(0);
      }
    }
  });

  it('rejects a short title inline and never calls onCreate', async () => {
    const onCreate = vi.fn<(p: CreateParams) => Promise<void>>();
    const { tree } = await renderModal({ onCreate });
    setInput(tree, 'Research campaign name or hypothesis', 'ab');
    await submit(tree);
    expect(tree.root.findByProps({ 'data-testid': 'campaign-title-error' }).props.children).toMatch(/at least 3/);
    expect(onCreate).not.toHaveBeenCalled();
    expect(tree.toJSON()).not.toBeNull();
  });

  it('rejects an empty title inline', async () => {
    const onCreate = vi.fn<(p: CreateParams) => Promise<void>>();
    const { tree } = await renderModal({ onCreate });
    setInput(tree, 'Research campaign name or hypothesis', '   ');
    await submit(tree);
    expect(tree.root.findByProps({ 'data-testid': 'campaign-title-error' })).toBeDefined();
    expect(onCreate).not.toHaveBeenCalled();
  });

  it('rejects an invalid custom molecule inline without falling back to the preset', async () => {
    const onCreate = vi.fn<(p: CreateParams) => Promise<void>>();
    const { tree } = await renderModal({ onCreate });
    setInput(tree, 'Custom chemical formula or SMILES', 'H2 O!!');
    await submit(tree);
    expect(tree.root.findByProps({ 'data-testid': 'campaign-molecule-error' })).toBeDefined();
    expect(onCreate).not.toHaveBeenCalled();
    // Still open, invalid value preserved (no silent preset substitution).
    expect(tree.root.findByProps({ 'aria-label': 'Custom chemical formula or SMILES' }).props.value).toBe('H2 O!!');
  });

  it('creates with the valid custom molecule (trimmed) and closes', async () => {
    const onCreate = vi.fn<(p: CreateParams) => Promise<void>>();
    const onClose = vi.fn(() => {});
    const { tree } = await renderModal({ onCreate, onClose });
    setInput(tree, 'Custom chemical formula or SMILES', '  CH4  ');
    await submit(tree);
    expect(onCreate).toHaveBeenCalledTimes(1);
    expect(onCreate.mock.calls[0][0]).toEqual({
      title: 'CH4 Ground State Optimization',
      targetMolecule: 'CH4',
      spendLimitUSD: 100,
      fresh: true,
    });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('keeps the modal OPEN and shows the server refusal verbatim on failure', async () => {
    const onCreate = vi.fn<(p: CreateParams) => Promise<void>>().mockRejectedValue(
      new Error('quota exhausted for tenant lab-a (verbatim)'),
    );
    const onClose = vi.fn(() => {});
    const { tree } = await renderModal({ onCreate, onClose });
    await submit(tree);
    expect(JSON.stringify(tree.toJSON())).toContain('quota exhausted for tenant lab-a (verbatim)');
    expect(tree.root.findByProps({ 'data-testid': 'campaign-create-error' })).toBeDefined();
    expect(onClose).not.toHaveBeenCalled();
    expect(tree.root.findByProps({ role: 'dialog' })).toBeDefined();
  });
});
