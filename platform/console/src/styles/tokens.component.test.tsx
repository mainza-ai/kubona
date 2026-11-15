import { afterEach, describe, expect, it } from 'vitest';
import './palette.css';
import './tokens.css';
import './axes.css';

function rootVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

afterEach(() => {
  delete document.documentElement.dataset.theme;
});

describe('token sheet in a real browser', () => {
  it('resolves the role-named radius and type ladder from the generated palette', () => {
    expect(rootVar('--radius-chip')).toBe('4px');
    expect(rootVar('--radius-row')).toBe('12px');
    expect(rootVar('--radius-card')).toBe('16px');
    expect(rootVar('--radius-sheet')).toBe('24px');
    expect(rootVar('--radius-capsule')).toBe('999px');

    expect(rootVar('--text-label')).toBe('12px');
    expect(rootVar('--text-label--line-height')).toBe('16px');
    expect(rootVar('--text-body-strong')).toBe('14px');
    expect(rootVar('--text-title-large')).toBe('24px');
    expect(rootVar('--text-display-large')).toBe('36px');
  });

  it('keeps the card overlay distinct from the raised surface', () => {
    const card = rootVar('--color-card');
    const raised = rootVar('--color-raised');
    expect(card).not.toBe('');
    expect(raised).not.toBe('');
    expect(card).not.toBe(raised);
    expect(rootVar('--k-color-card')).toBe(card);
    expect(rootVar('--k-color-surface')).toBe(rootVar('--color-surface'));
  });

  it('flips the semantic ladder under the light theme', () => {
    const darkSurface = rootVar('--color-surface');
    const darkCard = rootVar('--color-card');
    document.documentElement.dataset.theme = 'light';
    expect(rootVar('--color-surface')).toBe('#ffffff');
    expect(rootVar('--color-surface')).not.toBe(darkSurface);
    expect(rootVar('--k-color-card')).toBe(rootVar('--color-card'));
    expect(rootVar('--color-card')).not.toBe(darkCard);
  });
});
