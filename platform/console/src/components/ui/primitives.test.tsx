import { describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { Field, Select, Toggle } from './Forms';
import { Progress, SegmentedControl } from './Controls';
import { Button } from './Button';
import { Disclosure } from './Structure';
import { Menu, Tooltip } from './Overlays';

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map(textOf).join(' ');
}

describe('shared primitives', () => {
  it('associates field hint and error text for assistive technology', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<Field label="Campaign" hint="Visible to the lab" />);
    });
    const input = tree!.root.findByType('input');
    const describedBy = input.props['aria-describedby'];
    expect(describedBy).toBeTruthy();
    expect(textOf(tree!.toJSON())).toContain('Visible to the lab');
  });

  it('exposes native select, switch, and progress semantics', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <>
          <Select label="Backend" value="local" onChange={() => undefined} options={[{ value: 'local', label: 'Local' }]} />
          <Toggle label="Live telemetry" checked readOnly />
          <Progress label="Run" value={120} max={100} />
        </>,
      );
    });
    expect(tree!.root.findByType('select').props.value).toBe('local');
    expect(tree!.root.findByType('input').props.role).toBe('switch');
    expect(tree!.root.findByType('progress').props.value).toBe(100);
  });

  it('gives Button a safe default type and explicit busy/danger semantics', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <>
          <Button>Save</Button>
          <Button pending danger>Refund</Button>
        </>,
      );
    });
    const buttons = tree!.root.findAllByType('button');
    expect(buttons[0].props.type).toBe('button');
    expect(buttons[0].props.disabled).toBe(false);
    expect(buttons[1].props.type).toBe('button');
    expect(buttons[1].props.disabled).toBe(true);
    expect(buttons[1].props['aria-busy']).toBe(true);
    expect(buttons[1].props.className).toContain('k-button-danger');
  });

  it('implements the radiogroup roving tabindex and arrow-key selection', () => {
    const onChange = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <SegmentedControl
          label="View"
          value="a"
          onChange={onChange}
          segments={[{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }, { value: 'c', label: 'C', disabled: true }]}
        />,
      );
    });
    const radios = tree!.root.findAllByProps({ role: 'radio' });
    expect(radios[0].props.tabIndex).toBe(0);
    expect(radios[1].props.tabIndex).toBe(-1);
    act(() => {
      radios[0].props.onKeyDown({ key: 'ArrowRight', preventDefault: () => {} });
    });
    // Skips the disabled segment and announces the move.
    expect(onChange).toHaveBeenCalledWith('b');
  });

  it('describes the tooltip trigger, not its wrapper', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<Tooltip label="Shortcut"><button type="button">Hover</button></Tooltip>);
    });
    const trigger = tree!.root.findByType('button');
    const tooltip = tree!.root.findByProps({ role: 'tooltip' });
    expect(trigger.props['aria-describedby']).toBe(tooltip.props.id);
    expect(tooltip.props.children).toBe('Shortcut');
  });

  it('reports segment changes and keeps disclosure/menu content reachable', () => {
    const onChange = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <>
          <SegmentedControl label="View" value="a" onChange={onChange} segments={[{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }]} />
          <Disclosure summary="Details"><span>body</span></Disclosure>
          <Menu label="Actions"><button type="button">Run</button></Menu>
          <Tooltip label="Shortcut">Hover</Tooltip>
        </>,
      );
    });
    const segment = tree!.root.findAllByType('button').find((button) => textOf(button.props.children) === 'B');
    act(() => { (segment!.props as { onClick: () => void }).onClick(); });
    expect(onChange).toHaveBeenCalledWith('b');
    expect(JSON.stringify(tree!.toJSON())).toContain('body');
    expect(JSON.stringify(tree!.toJSON())).toContain('Shortcut');
  });
});
