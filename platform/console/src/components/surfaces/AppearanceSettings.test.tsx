import { describe, expect, it } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { AppearanceSettings } from './AppearanceSettings';
import {
  DEFAULT_APPEARANCE,
  TINT_OPTIONS,
  applyAppearanceToDocument,
} from '@/lib/appearance';

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map((item) => textOf(item)).join(' ');
  if (node && typeof node === 'object') {
    const kids = (node as { children?: unknown[] }).children ?? [];
    return kids.map((c) => textOf(c)).join(' ');
  }
  return '';
}

describe('AppearanceSettings & theme axes', () => {
  it('applies theme, tint, contrast, density, and reduced-motion attributes cleanly', () => {
    const dataset: Record<string, string> = {};
    const classes = new Set<string>();
    const fakeDoc = {
      documentElement: {
        dataset,
        classList: {
          add: (cls: string) => classes.add(cls),
          remove: (cls: string) => classes.delete(cls),
        },
      },
    } as unknown as Document;

    applyAppearanceToDocument(
      { theme: 'light', tint: 'lavender', contrast: true, density: 'compact', reduceMotion: true },
      fakeDoc,
    );
    expect(dataset.theme).toBe('light');
    expect(dataset.tint).toBe('lavender');
    expect(dataset.contrast).toBe('true');
    expect(dataset.density).toBe('compact');
    expect(dataset.reduceMotion).toBe('true');
    expect(classes.has('dark')).toBe(false);

    applyAppearanceToDocument(DEFAULT_APPEARANCE, fakeDoc);
    expect(dataset.theme).toBe('dark');
    expect(dataset.tint).toBeUndefined();
    expect(dataset.contrast).toBeUndefined();
    expect(dataset.density).toBeUndefined();
    expect(dataset.reduceMotion).toBeUndefined();
    expect(classes.has('dark')).toBe(true);
  });

  it('exposes all accent tint swatches and resets defaults', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<AppearanceSettings />);
    });

    const tintGroup = tree!.root.findByProps({ role: 'radiogroup', 'aria-label': 'Accent tint' });
    const radios = tintGroup.findAllByProps({ role: 'radio' });
    expect(radios).toHaveLength(TINT_OPTIONS.length);

    const lavenderBtn = radios.find((node) => textOf(node).includes('Lavender'))!;
    act(() => {
      lavenderBtn.props.onClick();
    });
    expect(lavenderBtn.props['aria-checked']).toBe(true);

    const lightBtn = tree!.root.findAllByProps({ role: 'radio' }).find((n) => n.props.children === 'Light paper')!;
    act(() => {
      lightBtn.props.onClick();
    });
    expect(lightBtn.props['aria-checked']).toBe(true);

    const resetBtn = tree!.root.findAllByType('button').find((b) => b.props.children === 'Reset defaults')!;
    act(() => {
      resetBtn.props.onClick();
    });
    const darkBtn = tree!.root.findAllByProps({ role: 'radio' }).find((n) => n.props.children === 'Dark graphite')!;
    expect(darkBtn.props['aria-checked']).toBe(true);
  });
});
