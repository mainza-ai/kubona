import { test, expect } from '@playwright/test';
import { mockApi } from './workbench-fixtures';

test('runtime transfer and DOM stay inside the recorded ceiling', async ({ page }) => {
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
  test.info().annotations.push({ type: 'runtime-budget', description: JSON.stringify(measured) });
  console.log(`runtime budget: ${JSON.stringify(measured)}`);
  expect(measured.transferBytes).toBeLessThanOrEqual(480 * 1024);
  expect(measured.visibleDom).toBeLessThanOrEqual(500);
  expect(measured.dom).toBeLessThanOrEqual(500);
  expect(measured.cards).toBeLessThanOrEqual(150);
});
