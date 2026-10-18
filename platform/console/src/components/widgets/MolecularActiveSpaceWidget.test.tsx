import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { MolecularActiveSpaceWidget } from './MolecularActiveSpaceWidget';

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

describe('MolecularActiveSpaceWidget', () => {
  it('renders orbital ladder with Fermi level divider and correct molecule name', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <MolecularActiveSpaceWidget
          moleculeName="LiH"
          numElectrons={4}
          numOrbitals={4}
          homoLumoGapHa={0.042}
        />
      );
    });

    const text = textOf(tree!.toJSON());
    expect(text).toContain('LiH');
    expect(text).toContain('Fermi Level');
    expect(text).toContain('HOMO-LUMO Gap');
    expect(text).toContain('HOMO');
    expect(text).toContain('LUMO');
    expect(text).toContain('Orbital Energy Ladder');
  });

  it('renders demo watermark when not backed by live session', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<MolecularActiveSpaceWidget />);
    });

    const text = textOf(tree!.toJSON());
    expect(text).toContain('synthetic demo — not session data');
  });

  it('switches between Orbitals and Occupancy tabs cleanly', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <MolecularActiveSpaceWidget
          moleculeName="H2"
          numElectrons={2}
          numOrbitals={2}
          homoLumoGapHa={0.05}
        />
      );
    });

    // Find the Occupancy tab button
    const buttons = tree!.root.findAllByType('button');
    const occBtn = buttons.find((b) => textOf(b.props.children).includes('Occupancy'));
    expect(occBtn).toBeDefined();

    await act(async () => {
      occBtn!.props.onClick();
    });

    const text = textOf(tree!.toJSON());
    expect(text).toContain('Active Space Configuration');
    expect(text).toContain('Frozen Core');
    expect(text).toContain('Multi-Reference Weight');
  });

  it('triggers onActiveSpaceChange when backed and slider changes', async () => {
    const onChange = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <MolecularActiveSpaceWidget
          moleculeName="LiH"
          numElectrons={4}
          numOrbitals={4}
          homoLumoGapHa={0.042}
          onActiveSpaceChange={onChange}
        />
      );
    });

    const inputs = tree!.root.findAllByType('input');
    expect(inputs.length).toBeGreaterThan(0);

    await act(async () => {
      inputs[0].props.onChange({ target: { value: '2' } });
    });

    expect(onChange).toHaveBeenCalled();
  });
});
