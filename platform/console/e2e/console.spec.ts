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
  await expect(page.getByText('Operator session')).toBeVisible();
}

async function ensureOperatorLoggedIn(page: Page) {
  const token = process.env.E2E_OPERATOR_TOKEN;
  test.skip(!token, 'E2E_OPERATOR_TOKEN not set: write-path operator setup skipped');

  await page.goto('/');
  await expectReady(page);
  await openLabs(page);
  await expect(page.locator('button').filter({ hasText: 'Default lab' }).first()).toBeVisible();

  // First run: the default lab has no admin key. Bootstrap it with the
  // operator token (held only in component state), then log in with the
  // browser-held Ed25519 key. On a later context the admin is already set,
  // so only the login button remains.
  const labButton = page.locator('button').filter({ hasText: 'Default lab' }).first();
  if (await labButton.count()) {
    await labButton.click();
  }
  if (await page.getByText('No admin registered', { exact: false }).isVisible().catch(() => false)) {
    await page.getByLabel('Operator token').last().fill(token!);
    await page.getByRole('button', { name: /Create browser key|Register this browser key/ }).click();
    await expect(page.getByText('No admin registered', { exact: false })).toHaveCount(0);
  }

  const login = page.getByRole('button', { name: /Log in with browser key|Session expired — log in again/ });
  if (await login.isVisible().catch(() => false)) {
    await login.click();
  }
  await expect(page.getByText(/Operator session/)).toBeVisible();
  await expect(page.getByText(/anonymous \(bearer only\)/)).toHaveCount(0);
}

test.describe('Kubona console against the real local stack', () => {
  test('boots, finds the backend, and reaches the core surfaces', async ({ page }) => {
    const errors = collectPageErrors(page);
    await page.goto('/');
    await expectReady(page);
    await page.keyboard.press('Tab');
    await expect(page.getByRole('link', { name: 'Skip to main content' })).toBeFocused();
    await page.keyboard.press('Enter');
    await expect(page.locator('#main-content')).toBeFocused();
    await expect(page.getByTestId('dock-arena')).toBeVisible();
    await expect(page.getByTestId('dock-arena')).toHaveAttribute('aria-current', 'page');
    await expect(page.getByRole('button', { name: 'Campaigns' })).toBeVisible();
    expect(errors).toEqual([]);
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
    await page.goto('/');
    await expectReady(page);
    await page.getByTestId('dock-recorder').click();
    await expect(page.getByText('Discovery Flight Recorder', { exact: false })).toBeVisible();
    await expect(page.getByText('Session portability', { exact: false })).toBeVisible();
  });

  test('operator login, local quantum dispatch, and codesign scoring', async ({ page }) => {
    await ensureOperatorLoggedIn(page);

    // A write that requires session authority must now succeed (the prior
    // fresh-DB path showed "Estimate refused: HTTP 401").
    await page.getByTestId('dock-mesh').click();
    await page.getByRole('button', { name: 'Sovereign Settings' }).click();
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
  });

  test('mobile shell keeps the copilot closed until opened as an overlay', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto('/');
    await expectReady(page);
    const openCopilot = page.getByRole('button', { name: /Kubona Copilot/ });
    await expect(openCopilot).toBeVisible();
    await expect(page.getByRole('button', { name: /Close Copilot/ })).toHaveCount(0);
    await openCopilot.click();
    await expect(page.getByRole('button', { name: /Close Copilot/ })).toBeVisible();
    await expect(page.getByRole('complementary', { name: 'Kubona co-pilot' })).toBeVisible();
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
