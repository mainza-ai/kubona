import { test, expect, type Page } from '@playwright/test';

function collectPageErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on('pageerror', (err) => errors.push(String(err)));
  return errors;
}

async function expectReady(page: Page) {
  await expect(page.getByRole('status', { name: /Backend status:/ })).toContainText(/ready/i);
}

async function openLabs(page: Page) {
  await page.getByTestId('dock-mesh').click();
  await page.getByRole('button', { name: 'Laboratories' }).click();
  await expect(page.getByText('Lab session')).toBeVisible();
}

async function ensureOperatorLoggedIn(page: Page) {
  const token = process.env.E2E_OPERATOR_TOKEN;
  test.skip(!token, 'E2E_OPERATOR_TOKEN not set: write-path operator setup skipped');

  await page.goto('/');
  await expectReady(page);
  await openLabs(page);

  // Create a fresh lab and register this browser key as its first admin.
  // This is the supported first-admin path and keeps the write test
  // repeatable when a prior browser key already owns another lab.
  const labId = `e2e-${Date.now().toString(36)}`;
  await page.getByLabel('New lab ID').fill(labId);
  await page.getByLabel('New lab display name').fill('E2E operator lab');
  await page.getByLabel('New lab operator token').fill(token!);
  const labCreated = page.waitForResponse((response) => (
    response.url().endsWith('/api/tenants')
    && response.request().method() === 'POST'
    && response.status() === 201
  ));
  const adminRegistered = page.waitForResponse((response) => (
    response.url().includes('/api/tenants/admin-key')
    && response.request().method() === 'POST'
    && response.status() === 200
  ));
  await page.getByRole('button', { name: 'Create lab + register admin' }).click();
  await labCreated;
  await adminRegistered;

  // A multi-lab store deliberately does not list labs anonymously; the
  // manual lab-id path is the supported login affordance.
  await page.getByRole('textbox', { name: 'Lab ID', exact: true }).fill(labId);
  await page.getByRole('button', { name: /Sign in with browser key|Session expired/ }).click();

  await expect(page.getByText(/Lab session/)).toBeVisible();
  await expect(page.getByText(/not signed in/)).toHaveCount(0);

  // The fresh lab has no sessions; create one with the authenticated cookie
  // and wait for the projection, so the dispatch step is not racing a
  // bootstrap that has not settled yet.
  const apiUrl = await page.evaluate(async () => {
    const config = await (await fetch('/api/config')).json() as { apiUrl?: string };
    return config.apiUrl || 'http://localhost:8080';
  });
  await page.evaluate(async (base) => {
    await fetch(`${base}/api/sessions`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        title: 'E2E operator campaign',
        target_molecule: 'H2',
        spend_limit_usd: 10,
        fresh: true,
      }),
    });
  }, apiUrl);
  await page.goto('/');
  await expectReady(page);
  const workbench = page.getByTestId('workbench');
  await expect(workbench).toBeVisible();
  await expect.poll(async () => Number(await workbench.getAttribute('data-object-count')), { timeout: 30_000 }).toBeGreaterThan(0);
}

test.describe('Kubona console against the real local stack', () => {
  test('boots, finds the backend, and reaches the core surfaces', async ({ page }) => {
    const errors = collectPageErrors(page);
    await page.goto('/');
    await expectReady(page);
    await page.keyboard.press('Tab');
    await expect(page.getByRole('link', { name: 'Skip to main content' })).toBeFocused();
    await page.keyboard.press('Enter');
    await expect(page.locator('#kubona-main')).toBeFocused();
    await expect(page.getByTestId('dock-arena')).toBeVisible();
    await expect(page.getByTestId('dock-arena')).toHaveAttribute('aria-current', 'page');
    await expect(page.getByRole('button', { name: 'Campaigns' })).toBeVisible();
    expect(errors).toEqual([]);
  });

  test('anonymous boot stays quiet: no expired banner and no outage toast', async ({ page }) => {
    await page.goto('/');
    await expectReady(page);
    await page.waitForTimeout(4500);
    await expect(page.getByText('Backend unreachable — retrying', { exact: false })).toHaveCount(0);
    await expect(page.getByTestId('auth-expired-banner')).toHaveCount(0);
    await expect(page.getByTestId('api-problem-banner')).toHaveCount(0);
  });

  test('old deep links resolve without a 404 or blank main', async ({ page }) => {
    const paths = [
      '/?surface=arena',
      '/?surface=arena&tab=proof-dag',
      '/?surface=recorder',
      '/?surface=manifold',
      '/?surface=fleet',
      '/?surface=proofs&tab=manuscripts',
      '/?surface=mesh&tab=settings',
    ];
    for (const path of paths) {
      await page.goto(path);
      await expect(page.locator('#kubona-main')).toBeVisible();
      await expect(page.getByText('404', { exact: true })).toHaveCount(0);
    }
  });

  test('interactive controls expose an accessible name', async ({ page }) => {
    await page.goto('/');
    await expectReady(page);
    const unnamed = await page.locator('button, a[href]').evaluateAll((nodes) =>
      nodes
        .filter((el) => el.getAttribute('aria-hidden') !== 'true')
        .filter((el) => {
          const name = (
            el.getAttribute('aria-label') ||
            el.getAttribute('title') ||
            el.textContent ||
            ''
          ).trim();
          return name.length === 0;
        })
        .map((el) => `${el.tagName}: ${el.outerHTML.slice(0, 180)}`),
    );
    expect(unnamed).toEqual([]);

    const unlabeledFields = await page.locator('input:not([type="hidden"]), select, textarea').evaluateAll((nodes) =>
      nodes
        .filter((el) => {
          const id = el.getAttribute('id');
          const labelled = id ? document.querySelector(`label[for="${CSS.escape(id)}"]`) : null;
          const name = (
            el.getAttribute('aria-label') ||
            el.getAttribute('title') ||
            el.getAttribute('placeholder') ||
            labelled?.textContent ||
            ''
          ).trim();
          return name.length === 0;
        })
        .map((el) => `${el.tagName}: ${el.outerHTML.slice(0, 180)}`),
    );
    expect(unlabeledFields).toEqual([]);
    await expect(page.locator('main')).toHaveCount(1);
    const missingAlt = await page.locator('img:not([alt])').count();
    expect(missingAlt).toBe(0);
  });

  test('recorder and session portability render without a blank state', async ({ page }) => {
    await page.goto('/?surface=recorder');
    await expectReady(page);
    await expect(page.getByText('Discovery Flight Recorder', { exact: false })).toBeVisible();
    await expect(page.getByText('Session portability', { exact: false })).toBeVisible();
  });

  test('operator login, local quantum dispatch, and codesign scoring', async ({ page }) => {
    await ensureOperatorLoggedIn(page);

    // A write that requires session authority must now succeed (the prior
    // fresh-DB path showed "Estimate refused: HTTP 401").
    await page.getByTestId('dock-mesh').click();
    await page.getByRole('button', { name: 'Sovereign Settings' }).click();
    await page.getByRole('button', { name: 'Quantum dispatch' }).click();
    await expect(page.getByText('Human dispatch')).toBeVisible();
    await page.getByRole('button', { name: /Estimate \(dry run\)/ }).click();
    await expect(page.getByText('Estimated cost:', { exact: false })).toBeVisible();
    await expect(page.getByText(/Dry run only/)).toBeVisible();

    await page.getByRole('button', { name: /^Dispatch/ }).click();
    await expect(page.getByText(/status:/)).toBeVisible();
    await expect(page.getByText(/Job /)).toBeVisible();

    await page.getByTestId('dock-proofs').click();
    await page.getByRole('button', { name: 'Circuit Codesign' }).click();
    await expect(page.getByText('TQ scoring', { exact: false })).toBeVisible();
    await page.getByRole('button', { name: 'Score', exact: true }).click();
    await expect(page.getByText('TQ = 50', { exact: true })).toBeVisible();

    // The shipped workflow sample must be valid against the live tool
    // registry: plan only, no execution.
    await page.getByTestId('dock-mesh').click();
    await page.getByRole('button', { name: 'Guardrails' }).click();
    await expect(page.getByText('Workflow planner / runner')).toBeVisible();
    await page.getByRole('button', { name: 'Plan (dry run)' }).click();
    await expect(page.getByText(/dry_run true · steps 1/)).toBeVisible();
  });

  test('mobile shell keeps the inspector closed until opened as an overlay', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto('/');
    await expectReady(page);
    const openInspector = page.getByRole('button', { name: /Inspector/ });
    await expect(openInspector).toBeVisible();
    await expect(page.getByRole('complementary', { name: 'Inspector' })).toHaveCount(0);
    await openInspector.click();
    await expect(page.getByRole('complementary', { name: 'Inspector' })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Details' })).toHaveAttribute('aria-selected', 'true');
    await page.getByRole('tab', { name: 'Copilot' }).click();
    await expect(page.getByRole('button', { name: /Close Copilot/ })).toBeVisible();
    await page.getByRole('button', { name: 'Close inspector' }).click();
    await expect(page.getByRole('complementary', { name: 'Inspector' })).toHaveCount(0);
  });

  test('command palette traps focus and Escape closes it', async ({ page }) => {
    await page.goto('/');
    await expectReady(page);
    await page.keyboard.press('Control+k');
    const dialog = page.getByRole('dialog', { name: 'Command palette' });
    await expect(dialog).toBeVisible();
    await expect(page.getByRole('textbox', { name: 'Search surfaces and actions' })).toBeFocused();
    for (let i = 0; i < 8; i += 1) {
      await page.keyboard.press('Tab');
      expect(await dialog.evaluate((el) => el.contains(document.activeElement))).toBe(true);
    }
    await page.keyboard.press('Escape');
    await expect(dialog).toHaveCount(0);
  });

  test('200% zoom equivalent keeps the shell inside the viewport', async ({ page }) => {
    await page.setViewportSize({ width: 720, height: 450 });
    await page.goto('/');
    await expectReady(page);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow).toBeLessThanOrEqual(1);
    await page.keyboard.press('Tab');
    await expect(page.getByRole('link', { name: 'Skip to main content' })).toBeFocused();
  });

  test('skip link reaches main content and forced colors keep the shell usable', async ({ page }) => {
    await page.emulateMedia({ forcedColors: 'active' });
    await page.goto('/');
    await expectReady(page);
    await page.keyboard.press('Tab');
    const skip = page.getByRole('link', { name: 'Skip to main content' });
    await expect(skip).toBeFocused();
    await page.keyboard.press('Enter');
    expect(await page.evaluate(() => document.activeElement?.id)).toBe('kubona-main');
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow).toBeLessThanOrEqual(1);
  });

  test('reduced motion disables ambient animation utilities', async ({ page }) => {
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await page.goto('/');
    await expectReady(page);
    const animation = await page.evaluate(() => {
      const probe = document.createElement('div');
      probe.className = 'animate-float-gentle';
      document.body.appendChild(probe);
      const name = getComputedStyle(probe).animationName;
      probe.remove();
      return name;
    });
    expect(animation).toBe('none');
  });

  test('backend refusal is visible instead of a fake empty state', async ({ page }) => {
    const errors = collectPageErrors(page);
    await page.route('**/api/llm/settings', (route) =>
      route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"forced"}' }),
    );
    await page.goto('/');
    await page.getByTestId('dock-mesh').click();
    await page.getByRole('button', { name: 'Sovereign Settings' }).click();
    await expect(page.getByText(/Settings load failed|Settings refused|backend returned nothing/i)).toBeVisible();
    expect(errors).toEqual([]);
  });
});
