import { test, expect } from '@playwright/test';
import { mockApi } from './workbench-fixtures';

test('dense workbench stays inside the static transfer and DOM ceiling', async ({ page }) => {
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
  expect(measured.totalBytes).toBeLessThanOrEqual(480 * 1024);
  expect(measured.dom).toBeLessThanOrEqual(500);
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
