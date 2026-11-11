import { test, expect, type Page, type Route } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { mockApi } from './workbench-fixtures';

async function audit(page: Page, label: string) {
  const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze();
  if (results.violations.length) {
    console.log(
      label,
      JSON.stringify(
        results.violations.map((v) => ({
          id: v.id,
          impact: v.impact,
          help: v.help,
          nodes: v.nodes.map((n) => ({ target: n.target, html: n.html, failure: n.failureSummary })),
        })),
        null,
        2,
      ),
    );
  }
  expect(results.violations, label).toEqual([]);
}

function initialSettings() {
  return {
    active_provider: 'nvidia',
    active_model: 'nvidia/llama-3.1-nemotron-70b-instruct',
    air_gapped: false,
    disable_dsml_fallback: false,
    temperature: 0.2,
    max_tokens: 4096,
    providers: {
      omlx: {
        type: 'omlx',
        name: 'oMLX Local',
        wire: 'openai_chat',
        base_url: 'http://127.0.0.1:8000/v1',
        default_model: 'local-qwen-32b',
        enabled: true,
        is_local: true,
        has_key: false,
        secret_source: 'none',
        key_status: 'not_required',
      },
      nvidia: {
        type: 'nvidia',
        name: 'NVIDIA NIM',
        wire: 'openai_chat',
        base_url: 'https://integrate.api.nvidia.com/v1',
        default_model: 'nvidia/llama-3.1-nemotron-70b-instruct',
        enabled: true,
        is_local: false,
        has_key: true,
        secret_source: 'plaintext_db',
        key_status: 'unverified',
      },
      openai: {
        type: 'openai',
        name: 'OpenAI',
        wire: 'openai_responses',
        base_url: 'https://api.openai.com/v1',
        default_model: 'gpt-4.1',
        enabled: true,
        is_local: false,
        has_key: true,
        secret_source: 'env',
        key_status: 'valid',
      },
    },
    role_assignments: {
      copilot: { provider: 'nvidia', model: 'nvidia/llama-3.1-nemotron-70b-instruct' },
      'epistemic-scribe': { provider: 'openai', model: 'gpt-4.1' },
      'ansatz-scout': { provider: 'nvidia', model: 'nvidia/llama-3.1-nemotron-70b-instruct' },
      'disconfirmation-officer': { provider: 'nvidia', model: 'nvidia/llama-3.1-nemotron-70b-instruct' },
      'literature-analyst': { provider: 'nvidia', model: 'unpriced-experimental-model' },
    },
  };
}

async function installLLMHandlers(page: Page) {
  let state = initialSettings();
  const savedPayloads: Array<Record<string, unknown>> = [];

  await mockApi(page, { loops: 2, jobs: 2, obligations: 1 });
  await page.route('**/api/llm/**', async (route: Route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const json = (body: unknown, status = 200) =>
      route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });

    if (path === '/api/llm/settings') {
      if (route.request().method() === 'POST' || route.request().method() === 'PUT') {
        const body = JSON.parse(route.request().postData() || '{}') as Record<string, unknown>;
        savedPayloads.push(body);
        state = {
          ...state,
          ...body,
          providers: (body.providers as typeof state.providers | undefined) ?? state.providers,
          role_assignments: (body.role_assignments as typeof state.role_assignments | undefined) ?? state.role_assignments,
        };
        return json({ status: 'saved', settings: state });
      }
      return json(state);
    }

    if (path === '/api/llm/models') {
      const provider = url.searchParams.get('provider') || state.active_provider;
      if (provider === 'omlx') {
        return json({
          provider: 'omlx',
          source: 'live',
          models: [
            {
              id: 'local-qwen-32b',
              provider: 'omlx',
              context_window: 32768,
              capabilities: { tools: true, reasoning: true, streaming: true },
              priced: true,
            },
          ],
        });
      }
      if (provider === 'openai') {
        return json({
          provider: 'openai',
          source: 'catalog',
          models: [
            {
              id: 'gpt-4.1',
              provider: 'openai',
              context_window: 128000,
              capabilities: { tools: true, vision: true, structured_output: true, streaming: true },
              priced: true,
              input_per_1k_usd: 0.002,
              output_per_1k_usd: 0.008,
            },
          ],
        });
      }
      return json({
        provider: 'nvidia',
        source: 'catalog',
        models: [
          {
            id: 'nvidia/llama-3.1-nemotron-70b-instruct',
            provider: 'nvidia',
            context_window: 131072,
            capabilities: { tools: true, reasoning: true, streaming: true },
            priced: true,
            input_per_1k_usd: 0.0008,
            output_per_1k_usd: 0.0012,
          },
          {
            id: 'unpriced-experimental-model',
            provider: 'nvidia',
            context_window: 32768,
            capabilities: { tools: false, streaming: true },
            priced: false,
          },
        ],
      });
    }

    if (path === '/api/llm/discover') {
      return json({
        servers: [
          {
            provider: 'omlx',
            name: 'oMLX Local',
            wire: 'openai_chat',
            base_url: 'http://127.0.0.1:8000/v1',
            port: 8000,
            models: ['local-qwen-32b'],
            context_window: 32768,
            source: 'probe',
          },
        ],
        ports_scanned: [8000, 11434, 1234],
        checked_at: '2026-11-11T12:00:00Z',
      });
    }

    if (path.startsWith('/api/llm/providers/') && path.endsWith('/verify')) {
      const provider = decodeURIComponent(path.split('/')[4] || 'nvidia');
      if (state.providers[provider as keyof typeof state.providers]) {
        state.providers[provider as keyof typeof state.providers].key_status = 'valid';
      }
      return json({
        verification: {
          ok: true,
          provider,
          wire: state.providers[provider as keyof typeof state.providers]?.wire || 'openai_chat',
          key_status: 'valid',
          latency_ms: 19,
          models: [
            { id: 'nvidia/llama-3.1-nemotron-70b-instruct', provider },
            { id: 'unpriced-experimental-model', provider },
          ],
          check_generation: 1,
        },
        settings: state,
      });
    }

    if (path === '/api/llm/benchmark') {
      return json({
        success: true,
        provider: 'openai',
        model: 'gpt-4.1',
        wire: 'openai_responses',
        latency_ms: 145,
        attempt_count: 1,
        cost_usd: 0.00042,
        response: 'pong',
      });
    }

    return json({});
  });

  return { getSavedPayloads: () => savedPayloads };
}

test('LLMSettings local discovery, provider verify, role options, mono-model warning, and a11y pass cleanly', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', (err) => errors.push(String(err)));
  await installLLMHandlers(page);

  await page.goto('/?surface=mesh&tab=settings');
  await expect(page.getByText('Sovereign Intelligence Configuration')).toBeVisible();
  await expect(page.getByText(/stored unsealed in SQLite/i)).toBeVisible();
  await expect(page.getByTestId('mono-model-tribunal-warning')).toBeVisible();
  await expect(page.getByText('Unpriced · loops blocked')).toBeVisible();

  await page.getByRole('button', { name: 'Scan local servers' }).click();
  await expect(page.getByTestId('discovered-server-omlx')).toBeVisible();
  await page.getByTestId('discovered-server-omlx').getByRole('button', { name: 'Attach' }).click();

  await page.getByRole('button', { name: 'Verify NVIDIA NIM' }).click();
  await expect(page.getByTestId('llm-verify-result-nvidia')).toContainText('Verified (openai_chat, 19 ms, 2 models)');

  await page.getByRole('button', { name: 'Configure epistemic-scribe sampling and fallbacks' }).click();
  await expect(page.getByLabel('epistemic-scribe temperature')).toBeVisible();

  await audit(page, 'llm-settings-rich');
  expect(errors).toEqual([]);
});

test('CommandDock ModelPicker switches provider tabs, filters models, and passes WCAG A/AA cleanly', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', (err) => errors.push(String(err)));
  const { getSavedPayloads } = await installLLMHandlers(page);

  await page.goto('/');
  await expect(page.getByTestId('workbench')).toBeVisible();

  await page.getByRole('button', { name: 'Quick switch model' }).click();
  const picker = page.getByRole('region', { name: 'Quick model switcher' });
  await expect(picker).toBeVisible();
  await expect(picker.getByText('unpriced · loops blocked')).toBeVisible();

  await picker.getByLabel('Search models').fill('nemotron');
  await expect(picker.locator('button[data-model-id="nvidia/llama-3.1-nemotron-70b-instruct"]')).toBeVisible();

  await audit(page, 'command-dock-model-picker');

  await picker.getByRole('tab', { name: /openai/i }).click();
  await picker.getByLabel('Search models').fill('');
  await picker.locator('button[data-model-id="gpt-4.1"]').click();

  await expect.poll(() => getSavedPayloads().length, { timeout: 10_000 }).toBeGreaterThanOrEqual(1);
  const latestSave = getSavedPayloads().at(-1) as { active_provider?: string; active_model?: string };
  expect(latestSave.active_provider).toBe('openai');
  expect(latestSave.active_model).toBe('gpt-4.1');
  expect(errors).toEqual([]);
});
