import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { CommandPalette } from './CommandPalette';

type PaletteProps = React.ComponentProps<typeof CommandPalette>;

/** Fresh callbacks per case so call counts never leak between tests. */
function props(overrides: Partial<PaletteProps> = {}): PaletteProps {
  return {
    isOpen: true,
    onClose: vi.fn(),
    onSelectSurface: vi.fn(),
    onNewCampaign: vi.fn(),
    onEmergencyAbort: vi.fn(),
    onOpenAbout: vi.fn(),
    ...overrides,
  };
}

describe('CommandPalette in a real browser', () => {
  it('does not render when isOpen is false', async () => {
    const screen = await render(<CommandPalette {...props({ isOpen: false })} />);

    expect(screen.container.childElementCount).toBe(0);
    expect(screen.container.querySelector('[role="dialog"]')).toBeNull();
  });

  it('renders surfaces, actions, and sub-tabs when isOpen is true', async () => {
    const screen = await render(<CommandPalette {...props()} />);

    await expect.element(screen.getByRole('dialog', { name: 'Command palette' })).toBeVisible();
    const text = screen.container.textContent ?? '';
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

  it('indexes workbench objects after two query characters', async () => {
    const onOpenObject = vi.fn();
    const screen = await render(
      <CommandPalette
        {...props({
          objects: [
            { id: 'measurement', label: 'Measurement', title: 'Classical anchor computed', kind: 'measurement' },
          ],
          onOpenObject,
        })}
      />,
    );

    const input = screen.getByLabelText('Search surfaces and actions');
    // The open effect lands focus in the same tick it clears the query; wait
    // for it so the typed query is not wiped by the reset.
    await expect.element(input).toHaveFocus();
    await input.fill('anchor');

    const result = screen.getByRole('button', { name: /Measurement: Classical anchor computed/ });
    await expect.element(result).toBeVisible();
    expect(screen.container.textContent).toContain('Measurement: Classical anchor computed');

    await result.click();
    expect(onOpenObject).toHaveBeenCalledWith('measurement');
  });

  it('filters results by query and selects item on click', async () => {
    const onSelect = vi.fn();
    const onClose = vi.fn();
    const screen = await render(
      <CommandPalette {...props({ onSelectSurface: onSelect, onClose })} />,
    );

    const input = screen.getByLabelText('Search surfaces and actions');
    await expect.element(input).toHaveFocus();
    await input.fill('tribunal');

    const result = screen.getByRole('button', { name: /Epistemic Tribunal Room/ });
    await expect.element(result).toBeVisible();
    expect(screen.container.textContent).toContain('Epistemic Tribunal Room');
    expect(screen.container.textContent).not.toContain('Hugging Face Model Hub');

    await result.click();

    expect(onSelect).toHaveBeenCalledWith('tribunal');
    expect(onClose).toHaveBeenCalled();
  });
});
