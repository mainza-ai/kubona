import { test, expect } from '@playwright/test';
import { mockApi } from './workbench-fixtures';

test('old surface and tab deep links resolve to a populated shell', async ({ page }) => {
  await mockApi(page, { loops: 2, jobs: 2, obligations: 1 });
  const cases: Array<{ path: string; active: string; header: boolean }> = [
    { path: '/?surface=arena', active: 'dock-arena', header: false },
    { path: '/?surface=arena&tab=proof-dag', active: 'dock-arena', header: false },
    { path: '/?surface=recorder', active: 'dock-arena', header: true },
    { path: '/?surface=manifold', active: 'dock-arena', header: true },
    { path: '/?surface=fleet', active: 'dock-mesh', header: true },
    { path: '/?surface=proofs&tab=manuscripts', active: 'dock-proofs', header: true },
    { path: '/?surface=mesh&tab=settings', active: 'dock-settings', header: true },
  ];
  for (const item of cases) {
    await page.goto(item.path);
    await expect(page.locator('#kubona-main')).toBeVisible();
    await expect(page.getByTestId(item.active)).toHaveAttribute('aria-current', 'page');
    if (item.header) await expect(page.getByTestId('campaign-header')).toBeVisible();
    await expect(page.getByText('404', { exact: true })).toHaveCount(0);
  }
});
