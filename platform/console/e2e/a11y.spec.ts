import { test, expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

async function audit(page: Page, label: string) {
  const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze();
  if (results.violations.length) {
    console.log(label, JSON.stringify(results.violations.map((v) => ({ id: v.id, impact: v.impact, nodes: v.nodes.length, help: v.help })), null, 2));
  }
  expect(results.violations, label).toEqual([]);
}

test('WCAG 2 A/AA automated audit on core surfaces', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('status', { name: /Backend status:/ })).toContainText(/ready/i);
  await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1);
  await expect(page.locator('main')).toHaveCount(1);
  await audit(page, 'arena');

  await page.getByTestId('dock-mesh').click();
  await page.getByRole('button', { name: 'Laboratories' }).click();
  await audit(page, 'labs');

  await page.getByRole('button', { name: 'Sovereign Settings' }).click();
  await audit(page, 'settings');

  await page.getByTestId('dock-proofs').click();
  await page.getByRole('button', { name: 'Circuit Codesign' }).click();
  await audit(page, 'codesign');
});
