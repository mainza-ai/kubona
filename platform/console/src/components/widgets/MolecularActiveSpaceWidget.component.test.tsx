import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import { MolecularActiveSpaceWidget } from './MolecularActiveSpaceWidget';

describe('MolecularActiveSpaceWidget in a real browser', () => {
  it('renders proposal counts and the recorded gap without inventing orbital energies', async () => {
    const screen = await render(
      <MolecularActiveSpaceWidget
        moleculeName="LiH"
        numElectrons={4}
        numOrbitals={4}
        homoLumoGapHa={0.042}
      />,
    );

    const text = screen.container.textContent ?? '';
    expect(text).toContain('LiH');
    expect(text).toContain('CASSCF(4e, 4o)');
    expect(text).toContain('HOMO-LUMO gap');
    expect(text).toContain('0.042 Ha');
    expect(text).toContain('Per-orbital energies are not shown');
    expect(text).not.toContain('Fermi Level');
    expect(text).not.toContain('Multi-Reference Weight');
  });

  it('renders an honest empty state when counts are absent', async () => {
    const screen = await render(<MolecularActiveSpaceWidget />);

    const text = screen.container.textContent ?? '';
    expect(text).toContain('No active-space counts are recorded for this campaign.');
    expect(text).not.toContain('LiH');
    expect(text).not.toContain('synthetic demo');
  });

  it('shows proposal counts when the gap is not recorded', async () => {
    const screen = await render(
      <MolecularActiveSpaceWidget moleculeName="H2" numElectrons={2} numOrbitals={2} />,
    );

    const text = screen.container.textContent ?? '';
    expect(text).toContain('H2');
    expect(text).toContain('HOMO-LUMO gap');
    expect(text).toContain('not recorded');
    expect(text).not.toContain('0.042');
  });

  it('reports the active space when a slider moves', async () => {
    const onChange = vi.fn();
    const screen = await render(
      <MolecularActiveSpaceWidget
        moleculeName="LiH"
        numElectrons={4}
        numOrbitals={4}
        homoLumoGapHa={0.042}
        onActiveSpaceChange={onChange}
      />,
    );

    const slider = screen.getByLabelText('Active electrons').element() as HTMLInputElement;
    expect(slider.value).toBe('4');

    // A real arrow key on the focused range input fires the browser's own
    // input/change events: 4 e → 2 e at step 2.
    slider.focus();
    await userEvent.keyboard('{ArrowLeft}');

    await expect.poll(() => onChange.mock.calls.length).toBeGreaterThan(0);
    expect(onChange).toHaveBeenCalledWith(2, 4);
    expect(slider.value).toBe('2');
    expect(screen.container.textContent).toContain('2 e / 4 o');
    expect(screen.container.textContent).toContain('CASSCF(2e, 4o)');
  });

  it('invokes onOpenObservatory CTA when provided on a backed widget', async () => {
    const onOpenObservatory = vi.fn();
    const screen = await render(
      <MolecularActiveSpaceWidget
        moleculeName="H2"
        numElectrons={2}
        numOrbitals={2}
        onOpenObservatory={onOpenObservatory}
      />,
    );

    const cta = screen.getByRole('button', { name: /Open 3D Observatory/ });
    await expect.element(cta).toBeVisible();
    await cta.click();
    expect(onOpenObservatory).toHaveBeenCalledTimes(1);
  });
});
