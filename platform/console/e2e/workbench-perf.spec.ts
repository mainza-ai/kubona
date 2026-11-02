import { test, expect } from '@playwright/test';
import type { Page } from '@playwright/test';
import { mockApi } from './workbench-fixtures';

// Opt-in browser evidence for the 300-object workbench. The mock keeps the
// measurement independent of a live database; set KUBONA_PERF=1 to run.
test.skip(!process.env.KUBONA_PERF, 'KUBONA_PERF not set: browser workbench performance evidence skipped');

async function measureWorkbench(page: Page, counts: { loops: number; jobs: number; obligations: number }, minObjects: number, minFps: number) {
  page.on('pageerror', (error) => console.log('PAGEERROR', error.message, error.stack));
  page.on('console', (message) => { if (message.type() === 'error') console.log('CONSOLE', message.text()); });
  await mockApi(page, counts);
  await page.goto('/');
  const workbench = page.getByTestId('workbench');
  await expect(workbench).toBeVisible();
  await expect.poll(async () => Number(await workbench.getAttribute('data-object-count')), { timeout: 20_000 }).toBeGreaterThanOrEqual(minObjects);
  const objectCount = Number(await workbench.getAttribute('data-object-count'));
  await page.waitForTimeout(750);

  const box = await workbench.boundingBox();
  expect(box).not.toBeNull();
  const centerX = box!.x + box!.width / 2;
  const centerY = box!.y + box!.height / 2;

  const measuring = page.evaluate(async () => {
    const frames: number[] = [];
    const started = performance.now();
    await new Promise<void>((resolve) => {
      const tick = (at: number) => {
        frames.push(at);
        if (at - started < 1500) requestAnimationFrame(tick);
        else resolve();
      };
      requestAnimationFrame(tick);
    });
    const elapsed = frames[frames.length - 1] - frames[0];
    const intervals = frames.slice(1).map((at, index) => at - frames[index]).sort((a, b) => a - b);
    const p95 = intervals[Math.min(intervals.length - 1, Math.floor(intervals.length * 0.95))] ?? 0;
    return { fps: frames.length / (elapsed / 1000), p95, frames: frames.length };
  });

  for (let index = 0; index < 36; index += 1) {
    await page.mouse.move(centerX, centerY);
    await page.mouse.wheel(0, index % 2 === 0 ? 140 : -140);
    await page.waitForTimeout(38);
  }

  const result = await measuring;
  test.info().annotations.push({ type: 'workbench-perf', description: JSON.stringify({ objectCount, ...result }) });
  console.log(`workbench perf (${objectCount} objects): ${result.fps.toFixed(1)} fps, p95 ${result.p95.toFixed(1)} ms, ${result.frames} frames`);
  expect(result.fps).toBeGreaterThanOrEqual(minFps);
}

test('100-object workbench sustains the interaction frame budget', async ({ page }) => {
  await measureWorkbench(page, { loops: 40, jobs: 40, obligations: 20 }, 100, 55);
});

test('300+ object workbench sustains the interaction frame budget', async ({ page }) => {
  await measureWorkbench(page, { loops: 160, jobs: 160, obligations: 32 }, 300, 30);
});
