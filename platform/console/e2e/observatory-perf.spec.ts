import { test, expect } from '@playwright/test';
import { mockApi } from './workbench-fixtures';

// Browser-level W5 proof: the 3D loop publishes its own frame counter, and
// stops scheduling frames when the tab is hidden or the surface is offscreen.
test('3D observatory stops rendering while hidden or offscreen', async ({ page }) => {
  await mockApi(page);
  await page.goto('/');
  await page.getByRole('button', { name: 'Open molecule object view' }).click();
  await page.getByRole('button', { name: '3D Observatory' }).click();

  const observatory = page.getByTestId('observatory-3d');
  await expect(observatory).toBeVisible();
  await expect(observatory).toHaveAttribute('data-render-state', /idle|active/);

  const frames = async () => Number(await observatory.getAttribute('data-render-frames'));
  const canvas = observatory.locator('canvas');
  const box = await canvas.boundingBox();
  if (!box) throw new Error('3D canvas has no bounds');
  const drag = async () => {
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.down();
    await page.mouse.move(box.x + box.width / 2 + 60, box.y + box.height / 2 + 30, { steps: 8 });
    await page.mouse.up();
    await page.waitForTimeout(350);
  };

  const before = await frames();
  await drag();
  await expect.poll(frames).toBeGreaterThan(before);

  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => true });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await expect(observatory).toHaveAttribute('data-render-state', 'hidden');
  const hiddenFrames = await frames();
  await page.waitForTimeout(450);
  expect(await frames()).toBe(hiddenFrames);

  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => false });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await drag();
  await expect.poll(frames).toBeGreaterThan(hiddenFrames);

  await observatory.evaluate((element) => {
    const node = element as HTMLElement;
    node.style.position = 'fixed';
    node.style.inset = 'auto';
    node.style.top = '-3000px';
    node.style.left = '0';
  });
  await expect(observatory).toHaveAttribute('data-render-state', 'offscreen');
  // Let the one frame that may already be in flight settle before sampling.
  await page.waitForTimeout(100);
  const offscreenFrames = await frames();
  await page.waitForTimeout(450);
  expect(await frames()).toBe(offscreenFrames);
});
