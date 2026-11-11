import { test, expect } from '@playwright/test';
import type { Page } from '@playwright/test';
import { mockApi } from './workbench-fixtures';

// Opt-in browser evidence for the 300-object workbench. The mock keeps the
// measurement independent of a live database; set KUBONA_PERF=1 to run.
test.skip(!process.env.KUBONA_PERF, 'KUBONA_PERF not set: browser workbench performance evidence skipped');

async function assertZeroCardOverlap(page: Page, viewport: { width: number; height: number }, label: string) {
  await page.setViewportSize(viewport);
  await page.waitForTimeout(120);
  const overlap = await page.evaluate(() => {
    const cards = Array.from(document.querySelectorAll('[data-testid="workbench"] .k-workbench-card'));
    const rects = cards
      .map((el) => el.getBoundingClientRect())
      .filter((r) => r.width > 0 && r.height > 0);
    let maxOverlapArea = 0;
    let overlappingPairs = 0;
    for (let i = 0; i < rects.length; i += 1) {
      for (let j = i + 1; j < rects.length; j += 1) {
        const a = rects[i];
        const b = rects[j];
        const ix = Math.max(0, Math.min(a.right, b.right) - Math.max(a.left, b.left));
        const iy = Math.max(0, Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top));
        const area = ix * iy;
        if (area > 4) {
          overlappingPairs += 1;
          if (area > maxOverlapArea) maxOverlapArea = area;
        }
      }
    }
    return { count: rects.length, overlappingPairs, maxOverlapArea };
  });
  expect(overlap.count).toBeGreaterThan(0);
  expect(overlap.overlappingPairs, `${label} overlap (${viewport.width}x${viewport.height})`).toBe(0);
}

async function measureWorkbench(page: Page, counts: { loops: number; jobs: number; obligations: number }, minObjects: number, minFps: number) {
  const pageErrors: string[] = [];
  const consoleErrors: string[] = [];
  page.on('pageerror', (error) => {
    pageErrors.push(error.message);
    console.log('PAGEERROR', error.message, error.stack);
  });
  page.on('console', (message) => {
    if (message.type() === 'error') {
      consoleErrors.push(message.text());
      console.log('CONSOLE', message.text());
    }
  });
  await page.setViewportSize({ width: 1440, height: 900 });
  await mockApi(page, counts);
  await page.goto('/');
  const workbench = page.getByTestId('workbench');
  await expect(workbench).toBeVisible();
  await expect.poll(async () => Number(await workbench.getAttribute('data-object-count')), { timeout: 20_000 }).toBeGreaterThanOrEqual(minObjects);
  const objectCount = Number(await workbench.getAttribute('data-object-count'));
  await page.waitForTimeout(750);

  await assertZeroCardOverlap(page, { width: 1440, height: 900 }, `${objectCount}-obj desktop`);
  await assertZeroCardOverlap(page, { width: 1024, height: 720 }, `${objectCount}-obj compact-desktop`);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.waitForTimeout(120);

  const box = await workbench.boundingBox();
  expect(box).not.toBeNull();
  const centerX = box!.x + box!.width / 2;
  const centerY = box!.y + box!.height / 2;

  await page.evaluate(() => {
    const samples: number[] = [];
    let moveStart = 0;
    const onCapture = () => { moveStart = performance.now(); };
    const onBubble = () => {
      if (moveStart > 0) samples.push(performance.now() - moveStart);
      moveStart = 0;
    };
    document.addEventListener('pointermove', onCapture, { capture: true });
    document.addEventListener('pointermove', onBubble, { capture: false });
    (window as unknown as { __kubonaPanProbe?: () => number[] }).__kubonaPanProbe = () => {
      document.removeEventListener('pointermove', onCapture, { capture: true });
      document.removeEventListener('pointermove', onBubble, { capture: false });
      return samples;
    };
  });

  const measuring = page.evaluate(async () => {
    const frames: number[] = [];
    const started = performance.now();
    await new Promise<void>((resolve) => {
      const tick = (at: number) => {
        frames.push(at);
        if (at - started < 1600) requestAnimationFrame(tick);
        else resolve();
      };
      requestAnimationFrame(tick);
    });
    const elapsed = frames[frames.length - 1] - frames[0];
    const intervals = frames.slice(1).map((at, index) => at - frames[index]).sort((a, b) => a - b);
    const over25 = intervals.filter((ms) => ms > 25).length;
    const p95 = intervals[Math.min(intervals.length - 1, Math.floor(intervals.length * 0.95))] ?? 0;
    return { fps: frames.length / (elapsed / 1000), p95, over25, frames: frames.length };
  });

  // Space+drag pan across the viewport, followed by wheel zoom stress.
  await page.keyboard.down('Space');
  await page.mouse.move(centerX - 160, centerY - 60);
  await page.mouse.down();
  for (let step = 1; step <= 24; step += 1) {
    await page.mouse.move(centerX - 160 + step * 12, centerY - 60 + (step % 4) * 6);
    await page.waitForTimeout(16);
  }
  await page.mouse.up();
  await page.keyboard.up('Space');

  for (let index = 0; index < 18; index += 1) {
    await page.mouse.move(centerX, centerY);
    await page.mouse.wheel(0, index % 2 === 0 ? 140 : -140);
    await page.waitForTimeout(36);
  }

  const result = await measuring;
  const panSamples = await page.evaluate(
    () => (window as unknown as { __kubonaPanProbe?: () => number[] }).__kubonaPanProbe?.() ?? [],
  );
  const sortedPan = [...panSamples].sort((a, b) => a - b);
  const panWorkP50 = sortedPan.length > 0 ? sortedPan[Math.floor(sortedPan.length * 0.5)] : 0;

  test.info().annotations.push({
    type: 'workbench-perf',
    description: JSON.stringify({ objectCount, ...result, panWorkP50, panSamples: sortedPan.length }),
  });
  console.log(
    `workbench perf (${objectCount} objects): ${result.fps.toFixed(1)} fps, p95 ${result.p95.toFixed(1)} ms, over25=${result.over25}, panWorkP50=${panWorkP50.toFixed(2)} ms (${sortedPan.length} moves), ${result.frames} frames`,
  );
  expect(sortedPan.length).toBeGreaterThanOrEqual(10);
  expect(panWorkP50).toBeLessThan(8);
  expect(result.over25).toBeLessThanOrEqual(4);
  expect(result.fps).toBeGreaterThanOrEqual(minFps);
  expect(pageErrors).toEqual([]);
  expect(consoleErrors).toEqual([]);
}

test('100-object workbench sustains the interaction frame budget', async ({ page }) => {
  await measureWorkbench(page, { loops: 40, jobs: 40, obligations: 20 }, 100, 45);
});

test('300+ object workbench sustains the interaction frame budget', async ({ page }) => {
  await measureWorkbench(page, { loops: 160, jobs: 160, obligations: 32 }, 300, 28);
});
