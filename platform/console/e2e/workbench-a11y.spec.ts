import { test, expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { mockApi } from './workbench-fixtures';

async function audit(page: Page, label: string) {
  const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze();
  if (results.violations.length) {
    console.log(label, JSON.stringify(results.violations.map((violation) => ({
      id: violation.id, impact: violation.impact, help: violation.help,
      nodes: violation.nodes.map((node) => ({ target: node.target, html: node.html, failure: node.failureSummary })),
    })), null, 2));
  }
  expect(results.violations, label).toEqual([]);
}

test('dense workbench canvas and list remain WCAG A/AA clean and ship worker-src self CSP', async ({ page }) => {
  await mockApi(page);
  const response = await page.goto('/');
  expect(response).not.toBeNull();
  const csp = response!.headers()['content-security-policy'] ?? '';
  expect(csp).toContain("worker-src 'self'");
  const workbench = page.getByTestId('workbench');
  await expect(workbench).toBeVisible();
  await expect.poll(async () => Number(await workbench.getAttribute('data-object-count')), { timeout: 20_000 }).toBeGreaterThanOrEqual(300);
  await audit(page, 'workbench-canvas-dark');

  await workbench.getByRole('button', { name: 'List' }).click();
  await expect(workbench.getByRole('button', { name: 'Canvas' })).toBeVisible();
  await audit(page, 'workbench-list-dark');
});

test('accessibility axes (light, high contrast, compact, reduced motion) remain WCAG A/AA clean', async ({ page }) => {
  await mockApi(page, { loops: 4, jobs: 4, obligations: 2 });
  await page.goto('/');
  const workbench = page.getByTestId('workbench');
  await expect(workbench).toBeVisible();

  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
  await page.waitForTimeout(250);
  await audit(page, 'workbench-light');

  await page.evaluate(() => {
    document.documentElement.dataset.contrast = 'true';
  });
  await page.waitForTimeout(250);
  await audit(page, 'workbench-light-high-contrast');

  await page.evaluate(() => {
    delete document.documentElement.dataset.theme;
    document.documentElement.dataset.contrast = 'true';
    document.documentElement.dataset.density = 'compact';
  });
  await page.waitForTimeout(250);
  const compactPad = await page.locator('.k-workbench-card').first().evaluate((el) => getComputedStyle(el).paddingTop);
  expect(compactPad).toBe('9px');
  await audit(page, 'workbench-dark-high-contrast-compact');

  await page.evaluate(() => {
    document.documentElement.dataset.reduceMotion = 'true';
  });
  const reducedTiming = await page.evaluate(() => {
    const row = document.querySelector('.k-workbench-part-row') ?? document.querySelector('.k-workbench-card');
    if (!row) return null;
    const style = getComputedStyle(row);
    return {
      transitionDuration: style.transitionDuration,
      animationDuration: style.animationDuration,
    };
  });
  expect(reducedTiming).not.toBeNull();
  const transSec = Number.parseFloat(reducedTiming!.transitionDuration);
  const animSec = Number.parseFloat(reducedTiming!.animationDuration);
  expect(Number.isFinite(transSec) && transSec <= 0.00005).toBe(true);
  expect(Number.isFinite(animSec) && animSec <= 0.00005).toBe(true);
  await audit(page, 'workbench-reduce-motion');

  await page.evaluate(() => {
    delete document.documentElement.dataset.contrast;
    delete document.documentElement.dataset.density;
    delete document.documentElement.dataset.reduceMotion;
  });
});

test('settings workspace is WCAG A/AA clean', async ({ page }) => {
  await mockApi(page, { loops: 2, jobs: 2, obligations: 1 });
  await page.goto('/?surface=mesh&tab=settings');
  await expect(page.getByTestId('settings-nav')).toBeVisible();
  await expect(page.getByText('Sovereign Intelligence Configuration')).toBeVisible();
  await audit(page, 'settings-workspace');
});

test('Light theme inversion, hover opacity invariance, zero card overlap, and zoom overview bond visibility', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (err) => pageErrors.push(err.message));

  await mockApi(page, { loops: 4, jobs: 4, obligations: 2 });
  await page.goto('/');
  const workbench = page.getByTestId('workbench');
  await expect(workbench).toBeVisible();

  const cards = workbench.locator('.k-workbench-card');
  await expect.poll(async () => await cards.count(), { timeout: 20_000 }).toBeGreaterThanOrEqual(4);

  // Default hybrid edge visibility toolbar control.
  await expect(workbench.getByRole('button', { name: 'Edges: hybrid' })).toBeVisible();

  // Semantic token inversion check (Zephium dark graphite default <-> Light Paper surfaces; no legacy #0E1117).
  const themeCheck = await page.evaluate(() => {
    const rootStyle = getComputedStyle(document.documentElement);
    const defaultCanvasVar = rootStyle.getPropertyValue('--color-canvas').trim().toLowerCase();
    const defaultBodyBg = getComputedStyle(document.body).backgroundColor;
    document.documentElement.dataset.theme = 'light';
    const lightCanvasVar = rootStyle.getPropertyValue('--color-canvas').trim().toLowerCase();
    const lightSurfaceVar = rootStyle.getPropertyValue('--color-surface').trim().toLowerCase();
    const lightCardBg = getComputedStyle(document.querySelector('.k-workbench-card')!).backgroundColor;
    const lightBodyBg = getComputedStyle(document.body).backgroundColor;
    delete document.documentElement.dataset.theme;
    return { defaultCanvasVar, defaultBodyBg, lightCanvasVar, lightSurfaceVar, lightCardBg, lightBodyBg };
  });
  expect(themeCheck.defaultCanvasVar).toBe('#1a1a1d');
  expect(themeCheck.lightCanvasVar).toBe('#f1f1f3');
  expect(['#fff', '#ffffff']).toContain(themeCheck.lightSurfaceVar);
  expect(themeCheck.defaultBodyBg).not.toBe('rgb(14, 17, 23)');
  expect(themeCheck.lightBodyBg).toBe('rgb(241, 241, 243)');
  expect(themeCheck.lightCardBg).not.toBe('rgb(14, 17, 23)');

  // Zero significant pairwise overlap across visible workbench cards.
  const overlaps = await workbench.evaluate((root) => {
    const rects = Array.from(root.querySelectorAll<HTMLElement>('.k-workbench-card')).map((el) => {
      const r = el.getBoundingClientRect();
      return {
        label: el.getAttribute('aria-label') || el.dataset.kind || 'card',
        left: r.left,
        top: r.top,
        right: r.right,
        bottom: r.bottom,
        width: r.width,
        height: r.height,
      };
    }).filter((r) => r.width > 1 && r.height > 1);

    const collisions: Array<{ a: string; b: string; area: number }> = [];
    for (let i = 0; i < rects.length; i++) {
      for (let j = i + 1; j < rects.length; j++) {
        const a = rects[i];
        const b = rects[j];
        const ix = Math.max(0, Math.min(a.right, b.right) - Math.max(a.left, b.left));
        const iy = Math.max(0, Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top));
        if (ix > 4 && iy > 4) {
          collisions.push({ a: a.label, b: b.label, area: Math.round(ix * iy) });
        }
      }
    }
    return collisions;
  });
  expect(overlaps).toEqual([]);

  // Hover opacity invariance: hovered and sibling cards stay opacity === 1.
  const firstCard = cards.first();
  await firstCard.hover();
  await page.waitForTimeout(100);
  const cardOpacities = await workbench.evaluate((root) =>
    Array.from(root.querySelectorAll<HTMLElement>('.k-workbench-card')).map((el) => ({
      label: el.getAttribute('aria-label') || 'card',
      opacity: getComputedStyle(el).opacity,
    })),
  );
  for (const item of cardOpacities) {
    expect(item.opacity, `${item.label} opacity during hover`).toBe('1');
  }

  // Zoomed-out overview (<= 50%) retains molecule bond distance (Å) visibly.
  const zoomOutBtn = workbench.getByRole('button', { name: 'Zoom out' });
  const zoomReadout = workbench.locator('.k-workbench-zoom');
  for (let i = 0; i < 8; i++) {
    const pctText = (await zoomReadout.innerText()).replace('%', '').trim();
    if (Number.parseInt(pctText, 10) <= 50) break;
    await zoomOutBtn.click();
  }
  const finalPct = Number.parseInt(((await zoomReadout.innerText()).replace('%', '').trim()), 10);
  expect(finalPct).toBeLessThanOrEqual(50);
  await expect(workbench.locator('.k-workbench-viewport')).toHaveAttribute('data-zoom-tier', 'overview');

  const moleculeCard = workbench.locator('.k-workbench-card[data-kind="molecule"]').first();
  await expect(moleculeCard).toBeVisible();
  const moleculeBondInfo = await moleculeCard.evaluate((card) => {
    const text = card.textContent ?? '';
    const match = /\d+(?:\.\d+)?\s*Å/.exec(text);
    const matchingEl = Array.from(card.querySelectorAll<HTMLElement>('p, dt, dd, span')).find((el) =>
      /\d+(?:\.\d+)?\s*Å/.test(el.textContent ?? ''),
    );
    const style = matchingEl ? getComputedStyle(matchingEl) : null;
    const rect = matchingEl ? matchingEl.getBoundingClientRect() : null;
    return {
      hasBondText: Boolean(match),
      bondText: match?.[0] ?? null,
      display: style?.display ?? null,
      opacity: style?.opacity ?? null,
      width: rect?.width ?? 0,
      height: rect?.height ?? 0,
    };
  });
  expect(moleculeBondInfo.hasBondText).toBe(true);
  expect(moleculeBondInfo.display).not.toBe('none');
  expect(moleculeBondInfo.opacity).toBe('1');
  expect(moleculeBondInfo.width).toBeGreaterThan(0);
  expect(moleculeBondInfo.height).toBeGreaterThan(0);
  expect(pageErrors).toEqual([]);
});
