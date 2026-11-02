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
  it('renders proposal counts and the recorded gap without inventing orbital energies', async () => {
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
    expect(text).toContain('CASSCF(4e, 4o)');
    expect(text).toContain('HOMO-LUMO gap');
    expect(text).toContain('0.042 Ha');
    expect(text).toContain('Per-orbital energies are not shown');
    expect(text).not.toContain('Fermi Level');
    expect(text).not.toContain('Multi-Reference Weight');
  });

  it('renders an honest empty state when counts are absent', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<MolecularActiveSpaceWidget />);
    });

    const text = textOf(tree!.toJSON());
    expect(text).toContain('No active-space counts are recorded for this campaign.');
    expect(text).not.toContain('LiH');
    expect(text).not.toContain('synthetic demo');
  });

  it('shows proposal counts when the gap is not recorded', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <MolecularActiveSpaceWidget moleculeName="H2" numElectrons={2} numOrbitals={2} />
      );
    });

    const text = textOf(tree!.toJSON());
    expect(text).toContain('H2');
    expect(text).toContain('HOMO-LUMO gap');
    expect(text).toContain('not recorded');
    expect(text).not.toContain('0.042');
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
