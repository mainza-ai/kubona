import { test, expect } from '@playwright/test';

const BASE = process.env.KUBONA_CONSOLE_URL || 'http://localhost:3000';

test('a ledger job event invalidates the jobs projection while the stream is live', async ({ page }) => {
  let jobsRequests = 0;
  let streamRequests = 0;
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    const json = (body: unknown) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
    if (path === '/api/config') return json({ apiUrl: BASE, workbench: true });
    if (path === '/api/health' || path === '/api/ready') return json({ ok: true, status: 'ready', ready: true });
    if (path === '/api/auth/whoami') return json({ authenticated: true, tenant_id: 'lab' });
    if (path === '/api/sessions') return json({ sessions: [{ id: 's1', title: 'Stream', status: 'active', created_at: '', updated_at: '' }] });
    if (path === '/api/sessions/s1') return json({ id: 's1', title: 'Stream', status: 'active', created_at: '', updated_at: '' });
    if (path === '/api/sessions/s1/campaign') return json({ session_id: 's1', research_goal: '', target_system: 'H2', quantum_factors: {}, classical_anchor: {}, trajectory: [], updated_at: '' });
    if (path === '/api/loops') return json({ loops: [] });
    if (path === '/api/jobs') { jobsRequests += 1; return json({ jobs: [] }); }
    if (path === '/api/events') return json({ events: [] });
    if (path === '/api/obligations/open') return json({ obligations: [] });
    if (path === '/api/tasks') return json({ tasks: [] });
    if (path === '/api/cost/summary') return json({ cap_usd: 100, spent_usd: 0, remaining_usd: 100, unmetered_count: 0, by_provider: {}, active_provider: 'local', local_bypass: true, heuristic: false, currency: 'USD', track_only: false });
    if (path === '/api/events/stream') {
      streamRequests += 1;
      const payload = {
        type: 'telemetry_tick', domain: 'ledger', kind: 'job_settled', revision: 1, session_id: 's1',
        events: [{ id: 1, event_type: 'job_settled', payload: { id: 'j1' }, session_id: 's1' }],
      };
      return route.fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body: `id: 1\nevent: telemetry\ndata: ${JSON.stringify(payload)}\n\n`,
      });
    }
    return json({});
  });
  await page.goto('/');
  await expect.poll(() => streamRequests, { timeout: 10_000 }).toBeGreaterThanOrEqual(1);
  await expect.poll(() => jobsRequests, { timeout: 10_000 }).toBeGreaterThanOrEqual(2);
});
