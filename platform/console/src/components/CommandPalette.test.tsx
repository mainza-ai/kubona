import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { CommandPalette } from './CommandPalette';

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

describe('CommandPalette', () => {
  it('does not render when isOpen is false', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <CommandPalette
          isOpen={false}
          onClose={vi.fn()}
          onSelectSurface={vi.fn()}
          onNewCampaign={vi.fn()}
          onEmergencyAbort={vi.fn()}
          onOpenAbout={vi.fn()}
        />
      );
    });
    expect(tree!.toJSON()).toBeNull();
  });

  it('renders surfaces, actions, and sub-tabs when isOpen is true', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <CommandPalette
          isOpen={true}
          onClose={vi.fn()}
          onSelectSurface={vi.fn()}
          onNewCampaign={vi.fn()}
          onEmergencyAbort={vi.fn()}
          onOpenAbout={vi.fn()}
        />
      );
    });

    const text = textOf(tree!.toJSON());
    expect(text).toContain('Active Research Arena');
    expect(text).toContain('4D Flight Recorder');
    expect(text).toContain('Epistemic Manifold');
    expect(text).toContain('Quantum Fleet Radar');
    expect(text).toContain('Merkle Proofs & Papers');
    expect(text).toContain('Epistemic Tribunal Room');
    expect(text).toContain('Generative UI Studio');
    expect(text).toContain('Autonomy Cockpit & Guardrails');
    expect(text).toContain('Hugging Face Model Hub');
    expect(text).toContain('Experimental Tenant Labs');
    expect(text).toContain('Sovereign Settings');
  });

  it('filters results by query and selects item on click', async () => {
    const onSelect = vi.fn();
    const onClose = vi.fn();

    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <CommandPalette
          isOpen={true}
          onClose={onClose}
          onSelectSurface={onSelect}
          onNewCampaign={vi.fn()}
          onEmergencyAbort={vi.fn()}
          onOpenAbout={vi.fn()}
        />
      );
    });

    const input = tree!.root.findByType('input');
    await act(async () => {
      input.props.onChange({ target: { value: 'tribunal' } });
    });

    const text = textOf(tree!.toJSON());
    expect(text).toContain('Epistemic Tribunal Room');
    expect(text).not.toContain('Hugging Face Model Hub');

    const button = tree!.root
      .findAllByType('button')
      .find((b) => textOf(b.props.children).includes('Epistemic Tribunal Room'));
    expect(button).toBeDefined();

    await act(async () => {
      button!.props.onClick();
    });

    expect(onSelect).toHaveBeenCalledWith('tribunal');
    expect(onClose).toHaveBeenCalled();
  });
});
