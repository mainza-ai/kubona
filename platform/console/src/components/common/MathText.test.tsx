import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { MathText } from './MathText';

function htmlOf(text: string): string {
  let tree: TestRenderer.ReactTestRenderer;
  act(() => {
    tree = TestRenderer.create(<MathText text={text} />);
  });
  const span = tree!.root.findByType('span');
  return String(span.props.dangerouslySetInnerHTML.__html);
}

describe('MathText markdown', () => {
  it('renders bold without leaking asterisks (user-reported PES case)', () => {
    const html = htmlOf('**Plan: 16-point H₂ PES scan**, STO-3G, with exact FCI.');
    expect(html).toContain('<strong');
    expect(html).toContain('Plan: 16-point');
    expect(html).not.toContain('**');
  });

  it('renders headers, bullets, ordered lists, quotes, and code', () => {
    const html = htmlOf('**Basis choice.** Minimal STO-3G\n\n- one variational parameter\n- warm-started\n\n1. first\n2. second\n\n> quoted\n\n`code`');
    expect(html).toContain('<ul');
    expect(html).toContain('<ol');
    expect(html).toContain('<code');
    expect(html).toContain('border-l-2');
  });

  it('renders GFM tables', () => {
    const html = htmlOf('| Quantity | Value |\n|---|---|\n| Minimum | −1.174476 Ha |');
    expect(html).toContain('<table');
    expect(html).toContain('−1.174476 Ha');
    expect(html).not.toContain('|---|');
  });

  it('keeps math rendering and never formats inside it', () => {
    const html = htmlOf('Energy $E_0$ with **bold** nearby.');
    expect(html).toContain('katex');
    expect(html).toContain('<strong');
  });

  it('escapes HTML from model output (no injection)', () => {
    const html = htmlOf('Hello <img src=x onerror=alert(1)> **bold**');
    expect(html).not.toContain('<img');
    expect(html).toContain('&lt;img');
    expect(html).toContain('<strong');
  });

  it('escapes HTML inside blockquotes (no injection)', () => {
    const html = htmlOf('> <script>alert(1)</script> **bold**');
    expect(html).not.toContain('<script');
    expect(html).toContain('&lt;script');
    expect(html).toContain('<strong');
  });

  it('leaves code fences literal', () => {
    const html = htmlOf('```\n**not bold** $nor math$\n```');
    expect(html).toContain('<pre');
    expect(html).not.toContain('<strong');
  });
});
