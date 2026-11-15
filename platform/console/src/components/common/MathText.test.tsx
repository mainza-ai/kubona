import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { MathText } from './MathText';

function renderMathText(text: string): TestRenderer.ReactTestRenderer {
  let tree: TestRenderer.ReactTestRenderer;
  act(() => {
    tree = TestRenderer.create(<MathText text={text} />);
  });
  return tree!;
}

function htmlOf(tree: TestRenderer.ReactTestRenderer): string {
  const span = tree.root.findByType('span');
  return String(span.props.dangerouslySetInnerHTML.__html);
}

describe('MathText markdown', () => {
  it('renders bold without leaking asterisks (user-reported PES case)', () => {
    const html = htmlOf(renderMathText('**Plan: 16-point H₂ PES scan**, STO-3G, with exact FCI.'));
    expect(html).toContain('<strong');
    expect(html).toContain('Plan: 16-point');
    expect(html).not.toContain('**');
  });

  it('renders headers, bullets, ordered lists, quotes, and code', () => {
    const html = htmlOf(renderMathText('**Basis choice.** Minimal STO-3G\n\n- one variational parameter\n- warm-started\n\n1. first\n2. second\n\n> quoted\n\n`code`'));
    expect(html).toContain('<ul');
    expect(html).toContain('<ol');
    expect(html).toContain('<code');
    expect(html).toContain('border-l-2');
  });

  it('renders GFM tables', () => {
    const html = htmlOf(renderMathText('| Quantity | Value |\n|---|---|\n| Minimum | −1.174476 Ha |'));
    expect(html).toContain('<table');
    expect(html).toContain('−1.174476 Ha');
    expect(html).not.toContain('|---|');
  });

  it('lazy-loads KaTeX and renders math without formatting inside it', async () => {
    const tree = renderMathText('Energy $E_0$ with **bold** nearby.');
    const start = Date.now();
    while (!htmlOf(tree).includes('katex') && Date.now() - start < 5000) {
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 10));
      });
    }
    const html = htmlOf(tree);
    expect(html).toContain('katex');
    expect(html).toContain('<strong');
  });

  it('escapes HTML from model output (no injection)', () => {
    const html = htmlOf(renderMathText('Hello <img src=x onerror=alert(1)> **bold**'));
    expect(html).not.toContain('<img');
    expect(html).toContain('&lt;img');
    expect(html).toContain('<strong');
  });

  it('escapes HTML inside blockquotes (no injection)', () => {
    const html = htmlOf(renderMathText('> <script>alert(1)</script> **bold**'));
    expect(html).not.toContain('<script');
    expect(html).toContain('&lt;script');
    expect(html).toContain('<strong');
  });

  it('leaves code fences literal', () => {
    const html = htmlOf(renderMathText('```\n**not bold** $nor math$\n```'));
    expect(html).toContain('<pre');
    expect(html).not.toContain('<strong');
  });
});
