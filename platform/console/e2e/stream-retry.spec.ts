import { test, expect } from '@playwright/test';

const BASE = process.env.KUBONA_CONSOLE_URL || 'http://localhost:3000';

test('the session stream retries after a refused connection instead of closing forever', async ({ page }) => {
  let streamRequests = 0;
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    const json = (body: unknown) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
    if (path === '/api/config') return json({ apiUrl: BASE, workbench: true });
    if (path === '/api/health' || path === '/api/ready') return json({ ok: true, status: 'ready', ready: true });
    if (path === '/api/auth/whoami') return json({ authenticated: true, tenant_id: 'lab' });
    if (path === '/api/sessions') return json({ sessions: [{ id: 's1', title: 'Stream', status: 'active', created_at: '', updated_at: '' }] });
    if (path === '/api/sessions/s1') return json({ id: 's1', title: 'Stream', status: 'active', created_at: '', updated_at: '' });
    if (path === '/api/loops') return json({ loops: [] });
    if (path === '/api/jobs') return json({ jobs: [] });
    if (path === '/api/events') return json({ events: [] });
    if (path === '/api/obligations/open') return json({ obligations: [] });
    if (path === '/api/tasks') return json({ tasks: [] });
    if (path === '/api/sessions/s1/campaign') return json({ session_id: 's1', research_goal: '', target_system: 'H2', quantum_factors: {}, classical_anchor: {}, trajectory: [], updated_at: '' });
    if (path === '/api/cost/summary') return json({ cap_usd: 100, spent_usd: 0, remaining_usd: 100, unmetered_count: 0, by_provider: {}, active_provider: 'local', local_bypass: true, heuristic: false, currency: 'USD', track_only: false });
    if (path === '/api/events/stream') {
      streamRequests += 1;
      if (streamRequests === 1) return route.fulfill({ status: 503, body: 'unavailable' });
      return route.fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body: 'event: connected\ndata: {"cursor":0}\n\n',
      });
    }
    return json({});
  });
  await page.goto('/');
  await page.waitForTimeout(4000);
  expect(streamRequests).toBeGreaterThanOrEqual(2);
});
