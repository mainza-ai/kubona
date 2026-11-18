import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { MathText } from './MathText';

// Rendered markdown/KaTeX is only observable in a real DOM: the component
// injects HTML, escapes model prose at the boundary, and lazy-loads KaTeX.
async function renderMarkdown(text: string, highlightNumbers: string[] = []): Promise<HTMLElement> {
  const screen = await render(<MathText text={text} highlightNumbers={highlightNumbers} />);
  return screen.container;
}

describe('MathText markdown in a real browser', () => {
  it('renders bold without leaking asterisks (user-reported PES case)', async () => {
    const container = await renderMarkdown('**Plan: 16-point H₂ PES scan**, STO-3G, with exact FCI.');
    const strong = container.querySelector('strong');
    expect(strong).not.toBeNull();
    expect(strong!.textContent).toBe('Plan: 16-point H₂ PES scan');
    expect(container.textContent).toContain('STO-3G, with exact FCI.');
    expect(container.textContent).not.toContain('**');
    expect(container.innerHTML).not.toContain('**');
  });

  it('renders headers, bullets, ordered lists, quotes, and code', async () => {
    const container = await renderMarkdown(
      '**Basis choice.** Minimal STO-3G\n\n- one variational parameter\n- warm-started\n\n1. first\n2. second\n\n> quoted\n\n`code`'
    );
    expect(container.querySelector('strong')!.textContent).toBe('Basis choice.');
    expect(container.textContent).toContain('Minimal STO-3G');

    const bulletList = container.querySelector('ul');
    expect(bulletList).not.toBeNull();
    expect(Array.from(bulletList!.querySelectorAll('li')).map((li) => li.textContent)).toEqual([
      '•one variational parameter',
      '•warm-started',
    ]);

    const orderedList = container.querySelector('ol');
    expect(orderedList).not.toBeNull();
    expect(Array.from(orderedList!.querySelectorAll('li')).map((li) => li.textContent)).toEqual(['first', 'second']);

    // Blockquote is a left-bordered div, not a <blockquote> element.
    const quote = container.querySelector('div.border-l-2');
    expect(quote).not.toBeNull();
    expect(quote!.textContent).toBe('quoted');

    const code = container.querySelector('code');
    expect(code).not.toBeNull();
    expect(code!.textContent).toBe('code');
  });

  it('renders GFM tables', async () => {
    const container = await renderMarkdown('| Quantity | Value |\n|---|---|\n| Minimum | −1.174476 Ha |');
    const table = container.querySelector('table');
    expect(table).not.toBeNull();
    expect(Array.from(table!.querySelectorAll('th')).map((th) => th.textContent)).toEqual(['Quantity', 'Value']);
    expect(Array.from(table!.querySelectorAll('td')).map((td) => td.textContent)).toEqual(['Minimum', '−1.174476 Ha']);
    expect(container.textContent).not.toContain('|---|');
  });

  it('marks unverified numbers in table cells too', async () => {
    const container = await renderMarkdown('| Quantity | Value |\n|---|---|\n| Energy | -1.13727 |', ['-1.13727']);
    expect(container.querySelector('table')).not.toBeNull();
    const marks = container.querySelectorAll('mark');
    expect(marks).toHaveLength(1);
    expect(marks[0].textContent).toBe('-1.13727');
  });

  it('lazy-loads KaTeX and renders math without formatting inside it', async () => {
    const container = await renderMarkdown('Energy $E_0$ with **bold** nearby.');
    // KaTeX is imported lazily, so the first paint is escaped source text.
    await expect.poll(() => container.querySelector('.katex') !== null).toBe(true);
    const math = container.querySelector('.katex')!;
    expect(math.textContent).not.toContain('$');
    expect(container.querySelector('strong')!.textContent).toBe('bold');
    expect(container.textContent).not.toContain('$E_0$');
  });

  it('escapes HTML from model output (no injection)', async () => {
    const container = await renderMarkdown('Hello <img src=x onerror=alert(1)> **bold**');
    // The real browser would fetch/execute an injected <img>; assert it never
    // reached the DOM, only the escaped text did.
    expect(container.querySelector('img')).toBeNull();
    expect(container.innerHTML).toContain('&lt;img');
    expect(container.textContent).toContain('<img src=x onerror=alert(1)>');
    expect(container.querySelector('strong')!.textContent).toBe('bold');
  });

  it('escapes HTML inside blockquotes (no injection)', async () => {
    const container = await renderMarkdown('> <script>alert(1)</script> **bold**');
    expect(container.querySelector('script')).toBeNull();
    expect(container.innerHTML).toContain('&lt;script');
    expect(container.textContent).toContain('<script>alert(1)</script>');
    expect(container.querySelector('strong')!.textContent).toBe('bold');
  });

  it('leaves code fences literal', async () => {
    const container = await renderMarkdown('```\n**not bold** $nor math$\n```');
    const pre = container.querySelector('pre');
    expect(pre).not.toBeNull();
    expect(pre!.textContent).toBe('**not bold** $nor math$');
    expect(container.querySelector('strong')).toBeNull();
    expect(container.querySelector('.katex')).toBeNull();
  });

  it('marks only the supplied unverified numbers in prose', async () => {
    const container = await renderMarkdown('Energy -1.13727 Ha and a second value 5.5 Ha.', ['-1.13727']);
    const marks = container.querySelectorAll('mark');
    expect(marks).toHaveLength(1);
    expect(marks[0].textContent).toBe('-1.13727');
    expect(container.textContent).toContain('5.5 Ha');
    // The unlisted 5.5 is not wrapped in a mark.
    expect(Array.from(marks).map((mark) => mark.textContent)).not.toContain('5.5');
  });

  it('does not mark numbers inside math placeholders', async () => {
    const container = await renderMarkdown('Energy $E_0$ is -1.13727 Ha.', ['-1.13727']);
    await expect.poll(() => container.querySelector('.katex') !== null).toBe(true);
    const marks = container.querySelectorAll('mark');
    expect(marks).toHaveLength(1);
    expect(marks[0].textContent).toBe('-1.13727');
  });

  it('keeps a wide numeric table inside its own scroll container', async () => {
    const screen = await render(
      <div data-testid="narrow-math" style={{ width: 320 }}>
        <MathText
          text={'| R (Å) | FCI | RHF |\n|---|---|---|\n| 1.0 | -7.78446028003123 [o3] | -7.767362 [o3] |\n| 4.0 | -7.784278178715385 [o9] | -7.624972 [o9] |'}
        />
      </div>,
    );
    const shell = screen.container.querySelector('[data-table-shell]') as HTMLElement | null;
    const scroller = screen.container.querySelector('[role="region"]') as HTMLElement | null;
    expect(shell).not.toBeNull();
    expect(scroller).not.toBeNull();
    expect(scroller!.scrollWidth).toBeGreaterThan(scroller!.clientWidth);
    expect(screen.getByTestId('narrow-math').element().scrollWidth).toBeLessThanOrEqual(320);

    const longCell = Array.from(screen.container.querySelectorAll('td')).find((cell) =>
      cell.textContent?.includes('−7.78446') || cell.textContent?.includes('-7.78446'),
    );
    expect(longCell).toBeTruthy();
    expect(longCell!.getAttribute('title')).toContain('7.78446028003123');
    expect(longCell!.className).toContain('text-right');
    expect(screen.container.querySelector('[data-copy-table]')).not.toBeNull();
  });
});
