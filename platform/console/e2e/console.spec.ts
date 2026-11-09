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
  const createdSessionId = await page.evaluate(async (base) => {
    const res = await fetch(`${base}/api/sessions`, {
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
    const body = await res.json() as { id?: string };
    return body.id || '';
  }, apiUrl);
  if (!createdSessionId) throw new Error('operator session creation returned no id');
  await page.goto('/');
  await expectReady(page);
  const workbench = page.getByTestId('workbench');
  await expect(workbench).toBeVisible();
  await expect.poll(async () => Number(await workbench.getAttribute('data-object-count')), { timeout: 30_000 }).toBeGreaterThan(0);
  return createdSessionId;
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

  test('agent workflow requires approval and runs through the Go rail', async ({ page }) => {
    await ensureOperatorLoggedIn(page);
    await page.getByTestId('dock-mesh').click();
    await page.getByRole('button', { name: 'Guardrails' }).click();
    await expect(page.getByText('Workflow planner / runner')).toBeVisible();
    const workflow = {
      name: 'e2e-agent-workflow',
      created_by_agent: true,
      budget_ceiling_usd: 0,
      steps: [{ id: 's1', tool: 'execution/list_provider_registry', args: {} }],
    };
    await page.getByLabel('Workflow JSON').fill(JSON.stringify(workflow, null, 2));
    await page.getByRole('button', { name: 'Approve current workflow' }).click();
    await expect(page.getByLabel('Workflow approval token')).toHaveValue(/.+/);
    await page.getByRole('button', { name: 'Run workflow…' }).click();
    const dialog = page.getByRole('alertdialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('textbox').fill(workflow.name);
    await dialog.getByRole('button', { name: 'Run workflow' }).click();
    await expect(page.getByText(/workflow .* completed 1/)).toBeVisible();
  });

  test('real LiH PES runs through the ledger and renders measured points', async ({ page }) => {
    const sessionId = await ensureOperatorLoggedIn(page);
    const apiUrl = await page.evaluate(async () => {
      const config = await (await fetch('/api/config')).json() as { apiUrl?: string };
      return config.apiUrl || 'http://localhost:8080';
    });
    const submitted = await page.evaluate(async ({ base, sid }) => {
      const res = await fetch(`${base}/api/experiments`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          session_id: sid,
          spec: {
            kind: 'pes', molecule: 'LiH', r_min: 1.4, r_max: 1.6, steps: 2,
            layers: 1, shots: 128, max_iterations: 2, full_space: false,
          },
          budget_usd: 1,
          request_id: `e2e-pes-${Date.now()}`,
        }),
      });
      return { status: res.status, body: await res.json() };
    }, { base: apiUrl, sid: sessionId });
    expect(submitted.status, JSON.stringify(submitted.body)).toBe(201);
    const jobId = submitted.body?.job?.id as string | undefined;
    expect(jobId).toBeTruthy();
    await expect.poll(async () => page.evaluate(async ({ base, sid, id }) => {
      const res = await fetch(`${base}/api/jobs/${id}?session_id=${sid}`, { credentials: 'include' });
      const body = await res.json() as { job?: { status?: string } };
      return body.job?.status || '';
    }, { base: apiUrl, sid: sessionId, id: jobId }), { timeout: 30_000 }).toBe('completed');
    await page.goto('/?surface=proofs&tab=manuscripts');
    await expect(page.getByText('PES points', { exact: true })).toBeVisible();
    await expect(page.getByText('1.4000', { exact: true })).toBeVisible();
  });

  test('real spin-chain and conjecture jobs complete through the Go rail', async ({ page }) => {
    const sessionId = await ensureOperatorLoggedIn(page);
    const apiUrl = await page.evaluate(async () => {
      const config = await (await fetch('/api/config')).json() as { apiUrl?: string };
      return config.apiUrl || 'http://localhost:8080';
    });
    const submit = async (spec: Record<string, unknown>, requestId: string) => {
      const out = await page.evaluate(async ({ base, sid, body }) => {
        const res = await fetch(`${base}/api/experiments`, {
          method: 'POST',
          credentials: 'include',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ session_id: sid, spec: body, budget_usd: 1, request_id: requestId }),
        });
        return { status: res.status, body: await res.json() };
      }, { base: apiUrl, sid: sessionId, body: spec });
      expect(out.status, JSON.stringify(out.body)).toBe(201);
      const id = out.body?.job?.id as string | undefined;
      expect(id).toBeTruthy();
      await expect.poll(async () => page.evaluate(async ({ base, sid, jobId }) => {
        const res = await fetch(`${base}/api/jobs/${jobId}?session_id=${sid}`, { credentials: 'include' });
        const body = await res.json() as { job?: { status?: string, result_json?: Record<string, unknown> } };
        return body.job?.status || '';
      }, { base: apiUrl, sid: sessionId, jobId: id }), { timeout: 30_000 }).toBe('completed');
      return id as string;
    };

    const spinId = await submit({
      kind: 'spin_chain', model: 'tfim', n_sites: 3, coupling: 1, field: 0.7,
      periodic: false, layers: 2, shots: 512, max_iterations: 20, seed: 5,
    }, `e2e-spin-${Date.now()}`);
    const spin = await page.evaluate(async ({ base, sid, id }) => {
      const res = await fetch(`${base}/api/jobs/${id}?session_id=${sid}`, { credentials: 'include' });
      return (await res.json()).job?.result_json as Record<string, unknown>;
    }, { base: apiUrl, sid: sessionId, id: spinId });
    expect(spin.kind).toBe('spin_chain');
    expect(spin.result_version).toBe('experiment-result-v1');
    expect(typeof spin.energy).toBe('number');
    expect(typeof spin.energy_exact).toBe('number');
    expect(spin.measured).toBe(true);
    expect(spin.result_receipt).toBeTruthy();

    const conjId = await submit({
      kind: 'conjecture', values: [1, 1, 2, 3, 5, 8, 13, 21, 34], holdout: 2,
    }, `e2e-conj-${Date.now()}`);
    const conj = await page.evaluate(async ({ base, sid, id }) => {
      const res = await fetch(`${base}/api/jobs/${id}?session_id=${sid}`, { credentials: 'include' });
      return (await res.json()).job?.result_json as Record<string, unknown>;
    }, { base: apiUrl, sid: sessionId, id: conjId });
    expect(conj.kind).toBe('conjecture');
    expect(conj.result_version).toBe('experiment-result-v1');
    expect(conj.candidate_count).toBeGreaterThanOrEqual(1);
    expect(conj.measured).toBe(false);
    expect(conj.claimable).toBe(false);
    expect(conj.result_receipt).toBeTruthy();
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

test.describe('result-surface browser contracts with deterministic fixtures', () => {
  test('claim paging and full-space status render from server payloads', async ({ page }) => {
    const claimsPageOne = {
      claims: [
        {
          id: 'clm-page-1', session_id: 'sess-1', turn_kind: 'vqe_run',
          statement: 'First measured claim.', alpha: -1, beta: -1, gamma: -1,
          verdict: 'verified', evidence_json: '{"job_id":"job-1"}',
          created_at: '2026-11-08T10:00:00Z',
        },
        {
          id: 'clm-page-2', session_id: 'sess-1', turn_kind: 'vqe_run',
          statement: 'Second measured claim.', alpha: -1, beta: -1, gamma: -1,
          verdict: 'verified', evidence_json: '{"job_id":"job-1"}',
          created_at: '2026-11-08T09:00:00Z',
        },
      ],
      offset: 0, limit: 50, next_offset: 2, has_more: true,
    };
    const claimsPageTwo = {
      claims: [{
        id: 'clm-page-3', session_id: 'sess-1', turn_kind: 'vqe_run',
        statement: 'Older claim from page two.', alpha: -1, beta: -1, gamma: -1,
        verdict: 'verified', evidence_json: '{"job_id":"job-older"}',
        created_at: '2026-11-07T09:00:00Z',
      }],
      offset: 2, limit: 50, next_offset: 3, has_more: false,
    };
    const campaign = {
      session_id: 'sess-1',
      research_goal: 'LiH full-space',
      target_system: 'LiH',
      quantum_factors: { target_molecule: 'LiH' },
      classical_anchor: {
        vqe_expectation_energy: -7.8816566228,
        fci_exact_energy: -7.882362286798725,
        discrepancy_mha: 0.7057,
      },
      trajectory: [],
      branches: [],
      epistemics: { status: 'COLLAPSED_VERIFIED' },
      notebook: {
        title: 'LiH full-space result',
        quantum_energy: -7.8816566228,
        classical_energy: -7.882362286798725,
        delta_mha: 0.7057,
        provenance_hash: 'abc123provenance',
        sections: [],
        chemical_accuracy: true,
      },
      measurement: { uncertainty_mha: 0.513, converged: true },
      pes_points: [{
        bond_distance: 1.5, energy: -7.8816, energy_exact: -7.8821,
        fci_exact_energy: -7.88236, delta_mha: 0.7, uncertainty_mha: 0.513,
        ci95_mha: 1.005, chemical_accuracy: true,
      }],
      updated_at: new Date().toISOString(),
    };
    const job = {
      id: 'job-full-space-1', session_id: 'sess-1', status: 'JOB_STATUS_COMPLETED',
      job_type: 'vqe_run', params_json: {}, created_at: '', completed_at: new Date().toISOString(),
      result_json: {
        energy: -7.8816566228, fci_exact_energy: -7.882362286798725,
        delta_mha: 0.7057, uncertainty: 0.000513, full_space_status: 'resource_exhausted',
        backend: 'qiskit-aer-simulator',
      },
    };

    await page.route('**/api/**', async (route) => {
      const url = new URL(route.request().url());
      const path = url.pathname;
      const fulfill = (body: unknown) => route.fulfill({
        status: 200, contentType: 'application/json', body: JSON.stringify(body),
      });
      if (path === '/api/sessions') {
        return fulfill({ sessions: [{
          id: 'sess-1', title: 'Fixture session', status: 'active', tenant_id: 'default',
          spend_limit_usd: 10, current_spend_usd: 0,
          created_at: '2026-11-08T00:00:00Z', updated_at: '2026-11-08T00:00:00Z',
        }] });
      }
      if (path === '/api/sessions/sess-1/campaign') return fulfill(campaign);
      if (path === '/api/sessions/sess-1/claims') {
        return fulfill(url.searchParams.get('offset') === '2' ? claimsPageTwo : claimsPageOne);
      }
      if (path.endsWith('/claims/clm-page-1/reviews') || path.endsWith('/claims/clm-page-2/reviews') || path.endsWith('/claims/clm-page-3/reviews')) {
        return fulfill({ reviews: [] });
      }
      if (path === '/api/jobs') return fulfill({ jobs: [job] });
      if (path === '/api/loops') return fulfill({ loops: [] });
      if (path === '/api/events') return fulfill({ events: [] });
      if (path === '/api/tasks') return fulfill({ tasks: [] });
      if (path === '/api/obligations' || path === '/api/obligations/open') return fulfill({ obligations: [] });
      if (path === '/api/cost/summary') return fulfill({ cap_usd: 10, spent_usd: 0, remaining_usd: 10, active_provider: 'local', currency: 'USD' });
      if (path === '/api/notebooks' || path === '/api/results/notebooks' || path.endsWith('/notebooks')) return fulfill({ notebooks: [] });
      if (path === '/api/access') {
        return fulfill({
          mode: 'single-lab', role: 'operator', single_lab: true, anonymous_chat: false,
          llm_local: false, campaign_create: false, auth_required: false,
        });
      }
      if (path === '/api/health') return fulfill({ status: 'ok' });
      if (path === '/api/ready') return fulfill({ status: 'ready' });
      if (path === '/api/auth/whoami') return fulfill({ authenticated: false });
      if (path === '/api/config') return fulfill({ apiUrl: 'http://localhost:8080' });
      return fulfill({});
    });

    await page.goto('/?surface=proofs&tab=manuscripts');
    await expect(page.getByText('Citable Results Studio', { exact: false })).toBeVisible();
    await expect(page.getByText('full-space resource exhausted', { exact: false })).toBeVisible();
    await expect(page.getByText('PES points', { exact: true })).toBeVisible();
    await expect(page.getByText('1.5000', { exact: true })).toBeVisible();
    await expect(page.getByText('1.005', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Load older claims' })).toBeVisible();
    await page.getByRole('button', { name: 'Load older claims' }).click();
    await expect(page.getByText('Older claim from page two.')).toBeVisible();
  });
});
