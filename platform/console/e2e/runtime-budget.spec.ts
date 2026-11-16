import { test, expect } from '@playwright/test';
import { mockApi } from './workbench-fixtures';

test('dense workbench stays inside the static transfer and DOM ceiling', async ({ page }) => {
  await page.addInitScript(() => {
    const perf = { lcp: 0, inp: 0 };
    (window as unknown as { __kubonaPerf: typeof perf }).__kubonaPerf = perf;
    try {
      new PerformanceObserver((list) => {
        for (const entry of list.getEntries()) perf.lcp = Math.max(perf.lcp, entry.startTime);
      }).observe({ type: 'largest-contentful-paint', buffered: true });
    } catch {
      /* browser without LCP */
    }
    try {
      new PerformanceObserver((list) => {
        for (const entry of list.getEntries() as PerformanceEventTiming[]) {
          if (typeof entry.duration === 'number') perf.inp = Math.max(perf.inp, entry.duration);
        }
      }).observe({ type: 'event', buffered: true, durationThreshold: 16 } as PerformanceObserverInit);
    } catch {
      /* browser without event timing */
    }
  });
  await mockApi(page);
  await page.goto('/');
  await expect(page.getByTestId('workbench')).toBeVisible();
  await page.waitForTimeout(750);

  const measured = await page.evaluate(() => {
    const resources = performance.getEntriesByType('resource') as PerformanceResourceTiming[];
    const nav = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined;
    const staticTransfer = resources
      .filter((entry) => !new URL(entry.name).pathname.startsWith('/api/'))
      .reduce((sum, entry) => sum + (entry.transferSize || entry.encodedBodySize || 0), 0);
    const apiTransfer = resources
      .filter((entry) => new URL(entry.name).pathname.startsWith('/api/'))
      .reduce((sum, entry) => sum + (entry.transferSize || entry.encodedBodySize || 0), 0);
    const transfer = staticTransfer + (nav?.transferSize ?? 0);
    const all = Array.from(document.querySelectorAll('*'));
    const visible = all.filter((element) => {
      const rect = element.getBoundingClientRect();
      return rect.width > 0 && rect.height > 0;
    });
    return {
      transferBytes: transfer,
      transferKB: Math.round(transfer / 1024),
      apiKB: Math.round(apiTransfer / 1024),
      dom: all.length,
      visibleDom: visible.length,
      cards: document.querySelectorAll('.k-workbench-card').length,
      placeholders: document.querySelectorAll('.k-workbench-placeholder').length,
    };
  });
  await page.mouse.click(24, 24);
  await page.waitForTimeout(250);
  const perf = await page.evaluate(() => {
    const p = (window as unknown as { __kubonaPerf?: { lcp: number; inp: number } }).__kubonaPerf;
    const memory = (performance as unknown as { memory?: { usedJSHeapSize: number } }).memory;
    const lcpEntries = performance.getEntriesByType('largest-contentful-paint') as PerformanceEntry[];
    const lcpFallback = lcpEntries.length > 0 ? lcpEntries[lcpEntries.length - 1].startTime : 0;
    return {
      lcpMs: Math.round(p?.lcp || lcpFallback),
      inpMs: Math.round(p?.inp ?? 0),
      heapMB: memory ? Math.round(memory.usedJSHeapSize / (1024 * 1024)) : null,
      requests: performance.getEntriesByType('resource').length,
    };
  });
  test.info().annotations.push({ type: 'runtime-budget', description: JSON.stringify(measured) });
  test.info().annotations.push({ type: 'runtime-perf', description: JSON.stringify(perf) });
  console.log(`runtime budget: ${JSON.stringify(measured)}`);
  // DOM ceilings re-baselined 2026-11-14 after the needs tray, status strip,
  // run-parts lane, and workbench toolbar landed: dense fixture measured 936
  // visible nodes (CI measured 1,122 on a later run). 1,250 keeps headroom
  // while still refusing an unbounded render; the transfer ceiling is unchanged.
  expect(measured.transferBytes).toBeLessThanOrEqual(480 * 1024);
  expect(measured.visibleDom).toBeLessThanOrEqual(1250);
  expect(measured.dom).toBeLessThanOrEqual(1250);
  expect(measured.cards).toBeLessThanOrEqual(150);
  expect(perf.lcpMs).toBeGreaterThan(0);
  expect(perf.lcpMs).toBeLessThanOrEqual(4000);
  expect(perf.inpMs).toBeLessThanOrEqual(500);
  if (perf.heapMB !== null) expect(perf.heapMB).toBeLessThanOrEqual(250);
  expect(perf.requests).toBeLessThanOrEqual(120);
});

test('normal boot stays inside the all-resource transfer ceiling', async ({ page }) => {
  await mockApi(page, { loops: 8, jobs: 8, obligations: 4 });
  await page.goto('/');
  await expect(page.getByTestId('workbench')).toBeVisible();
  await page.waitForTimeout(500);
  const measured = await page.evaluate(() => {
    const resources = performance.getEntriesByType('resource') as PerformanceResourceTiming[];
    const nav = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined;
    const total = resources.reduce((sum, entry) => sum + (entry.transferSize || entry.encodedBodySize || 0), 0) + (nav?.transferSize ?? 0);
    const all = Array.from(document.querySelectorAll('*')).length;
    return { totalBytes: total, totalKB: Math.round(total / 1024), dom: all };
  });
  test.info().annotations.push({ type: 'runtime-total-budget', description: JSON.stringify(measured) });
  console.log(`normal boot budget: ${JSON.stringify(measured)}`);
  // Normal boot measured 651 nodes with the 2026-11-14 shell; 800 is the
  // reviewed ceiling (transfer ceiling unchanged).
  expect(measured.totalBytes).toBeLessThanOrEqual(480 * 1024);
  expect(measured.dom).toBeLessThanOrEqual(800);
});


test('settings keeps a single-column DOM under the budget', async ({ page }) => {
  await mockApi(page, { loops: 4, jobs: 4, obligations: 2 });
  await page.goto('/?surface=mesh&tab=settings');
  await expect(page.getByTestId('campaign-header')).toBeVisible();
  await expect(page.getByTestId('settings-nav')).toBeVisible();
  await expect(page.getByText('Sovereign Intelligence Configuration')).toBeVisible();
  await page.waitForTimeout(300);
  const measured = await page.evaluate(() => {
    const all = Array.from(document.querySelectorAll('*'));
    const visible = all.filter((element) => {
      const rect = element.getBoundingClientRect();
      return rect.width > 0 && rect.height > 0;
    });
    return { dom: all.length, visibleDom: visible.length };
  });
  test.info().annotations.push({ type: 'settings-budget', description: JSON.stringify(measured) });
  console.log(`settings budget: ${JSON.stringify(measured)}`);
  expect(measured.visibleDom).toBeLessThanOrEqual(800);
});
