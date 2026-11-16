import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import { Field, Select, Toggle } from './Forms';
import { Progress, SegmentedControl } from './Controls';
import { Button } from './Button';
import { Disclosure } from './Structure';
import { Menu, Tooltip } from './Overlays';

describe('shared primitives in a real browser', () => {
  it('associates field hint and error text for assistive technology', async () => {
    const screen = await render(<Field label="Campaign" hint="Visible to the lab" />);
    const input = screen.getByLabelText('Campaign').element() as HTMLInputElement;
    const describedBy = input.getAttribute('aria-describedby');

    expect(describedBy).toBeTruthy();
    // The hint is the element the input actually points at, not just any text.
    const description = document.getElementById(describedBy!);
    expect(description).not.toBeNull();
    expect(description!.textContent).toBe('Visible to the lab');
    expect(description!.className).toBe('k-field-hint');
  });

  it('exposes native select, switch, and progress semantics', async () => {
    const screen = await render(
      <>
        <Select label="Backend" value="local" onChange={() => undefined} options={[{ value: 'local', label: 'Local' }]} />
        <Toggle label="Live telemetry" checked readOnly />
        <Progress label="Run" value={120} max={100} />
      </>,
    );

    const select = screen.getByLabelText('Backend').element() as HTMLSelectElement;
    expect(select.value).toBe('local');

    const toggle = screen.getByRole('switch').element() as HTMLInputElement;
    expect(toggle.checked).toBe(true);

    const progress = screen.getByRole('progressbar').element() as HTMLProgressElement;
    // The browser clamps the rendered value to max; no invented overrun.
    expect(progress.value).toBe(100);
    expect(progress.max).toBe(100);
  });

  it('gives Button a safe default type and explicit busy/danger semantics', async () => {
    const screen = await render(
      <>
        <Button>Save</Button>
        <Button pending danger>Refund</Button>
      </>,
    );

    const save = screen.getByRole('button', { name: 'Save' }).element() as HTMLButtonElement;
    const refund = screen.getByRole('button', { name: 'Refund' }).element() as HTMLButtonElement;

    expect(save.type).toBe('button');
    expect(save.disabled).toBe(false);
    expect(refund.type).toBe('button');
    expect(refund.disabled).toBe(true);
    expect(refund.getAttribute('aria-busy')).toBe('true');
    expect(refund.className).toContain('k-button-danger');
  });

  it('implements the radiogroup roving tabindex and arrow-key selection', async () => {
    const onChange = vi.fn();
    const screen = await render(
      <SegmentedControl
        label="View"
        value="a"
        onChange={onChange}
        segments={[{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }, { value: 'c', label: 'C', disabled: true }]}
      />,
    );
    const radios = screen.getByRole('radio');

    expect(radios.nth(0).element().getAttribute('tabindex')).toBe('0');
    expect(radios.nth(1).element().getAttribute('tabindex')).toBe('-1');
    expect((radios.nth(2).element() as HTMLButtonElement).disabled).toBe(true);

    await radios.nth(0).click();
    await userEvent.keyboard('{ArrowRight}');

    // Skips the disabled segment and moves both selection and focus.
    expect(onChange).toHaveBeenLastCalledWith('b');
    await expect.element(radios.nth(1)).toHaveFocus();
  });

  it('describes the tooltip trigger, not its wrapper', async () => {
    const screen = await render(
      <Tooltip label="Shortcut"><button type="button">Hover</button></Tooltip>,
    );
    const trigger = screen.getByRole('button', { name: 'Hover' }).element();
    const tooltip = screen.getByRole('tooltip').element();

    expect(trigger.getAttribute('aria-describedby')).toBe(tooltip.id);
    expect(tooltip.textContent).toBe('Shortcut');
    // The description belongs to the focusable trigger, not the span wrapper.
    expect(trigger.parentElement?.getAttribute('aria-describedby')).toBeNull();
  });

  it('reports segment changes and keeps disclosure/menu content reachable', async () => {
    const onChange = vi.fn();
    const screen = await render(
      <>
        <SegmentedControl label="View" value="a" onChange={onChange} segments={[{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }]} />
        <Disclosure summary="Details"><span>body</span></Disclosure>
        <Menu label="Actions"><button type="button">Run</button></Menu>
        <Tooltip label="Shortcut">Hover</Tooltip>
      </>,
    );

    await screen.getByRole('radio', { name: 'B' }).click();
    expect(onChange).toHaveBeenCalledWith('b');

    const disclosure = screen.getByText('Details').element().closest('details') as HTMLDetailsElement;
    expect(disclosure.open).toBe(false);
    await screen.getByText('Details').click();
    await expect.poll(() => disclosure.open).toBe(true);
    await expect.element(screen.getByText('body')).toBeVisible();

    await screen.getByText('Actions').click();
    await expect.element(screen.getByRole('menu').getByRole('button', { name: 'Run' })).toBeVisible();

    expect(screen.container.textContent).toContain('Shortcut');
  });
});
