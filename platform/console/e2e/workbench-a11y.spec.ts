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

test('dense workbench canvas and list remain WCAG A/AA clean', async ({ page }) => {
  await mockApi(page);
  await page.goto('/');
  const workbench = page.getByTestId('workbench');
  await expect(workbench).toBeVisible();
  await expect.poll(async () => Number(await workbench.getAttribute('data-object-count')), { timeout: 20_000 }).toBeGreaterThanOrEqual(300);
  await audit(page, 'workbench-canvas');

  await workbench.getByRole('button', { name: 'List' }).click();
  await expect(workbench.getByRole('button', { name: 'Canvas' })).toBeVisible();
  await audit(page, 'workbench-list');
});

test('settings workspace is WCAG A/AA clean', async ({ page }) => {
  await mockApi(page, { loops: 2, jobs: 2, obligations: 1 });
  await page.goto('/?surface=mesh&tab=settings');
  await expect(page.getByTestId('settings-nav')).toBeVisible();
  await expect(page.getByText('Sovereign Intelligence Configuration')).toBeVisible();
  await audit(page, 'settings-workspace');
});
