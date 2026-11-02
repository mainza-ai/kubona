import { test, expect } from '@playwright/test';

const BASE = process.env.KUBONA_CONSOLE_URL || 'http://localhost:3000';

test('anonymous read-only boot is quiet: no false outage toast or expired banner', async ({ page }) => {
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === '/api/config') {
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ apiUrl: BASE, workbench: true }) });
    }
    if (path === '/api/health' || path === '/api/ready') {
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true, status: 'ready', ready: true }) });
    }
    return route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'unauthorized', message: 'login required' }),
    });
  });
  await page.goto('/');
  await page.waitForTimeout(4500);
  await expect(page.getByText('Backend unreachable — retrying', { exact: false })).toHaveCount(0);
  await expect(page.getByTestId('auth-expired-banner')).toHaveCount(0);
  await expect(page.getByTestId('api-problem-banner')).toHaveCount(0);
});
