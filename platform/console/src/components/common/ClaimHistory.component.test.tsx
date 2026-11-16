import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { ClaimHistory } from './ClaimHistory';

type Call = { url: string; method: string; body: Record<string, unknown> | null };

const claim = {
  id: 'clm-test-0001',
  session_id: 's1',
  turn_kind: 'vqe_run',
  statement: 'VQE measured -1.1378 Ha vs FCI -1.1372 Ha.',
  alpha: -1,
  beta: -1,
  gamma: -1,
  verdict: 'verified',
  evidence_json: JSON.stringify({ job_id: 'job-12345678' }),
  created_at: '2026-11-08T10:00:00Z',
};

const reviewRecord = {
  id: 'rev-test-0001',
  session_id: 's1',
  claim_id: claim.id,
  decision: 'accepted',
  reason: 'uncertainty reviewed',
  reviewer: 'operator',
  evidence_json: '{}',
  created_at: '2026-11-08T11:00:00Z',
};

const provenance = {
  ok: true,
  claim: { id: claim.id, session_id: 's1', turn_kind: claim.turn_kind, statement: claim.statement, verdict: claim.verdict },
  evidence: [
    {
      claim_id: claim.id,
      kind: 'job_result',
      job_id: 'job-12345678',
      result_sha256: 'sha256:abcdef0123456789',
      receipt_entry_hash: '',
      source_ref: '',
      created_at: '2026-11-08T10:01:00Z',
    },
  ],
  summary: { measured: 1, general_knowledge: false, complete: true },
};

function bodyText(): string {
  return document.body.textContent ?? '';
}

function ok(data: unknown) {
  return { ok: true, status: 200, json: async () => data };
}

function refused(status = 404) {
  return { ok: false, status, json: async () => ({ message: 'refused' }), text: async () => 'refused' };
}

function claimPage(claims: unknown[], offset: number, hasMore: boolean) {
  return { claims, offset, limit: 50, next_offset: offset + claims.length, has_more: hasMore };
}

type Routes = {
  claims: (offset: number) => unknown;
  reviews?: () => unknown;
  provenance?: () => unknown;
  review?: () => unknown;
  /** Transport failure on the decision write, not a daemon refusal. */
  reviewThrows?: boolean;
  ledger?: () => unknown;
  chat?: () => unknown;
};

/** Stub the daemon by URL/method and record what the surface asked for. */
function stubDaemon(routes: Routes) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const method = (init?.method ?? 'GET').toUpperCase();
    const body = init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : null;
    const target = String(url);
    calls.push({ url: target, method, body });

    if (method === 'POST' && target.includes('/api/ledger/append')) {
      return ok(routes.ledger ? routes.ledger() : { ok: true, record_hash: 'rec-review' });
    }
    if (method === 'POST' && target.includes('/api/chat/messages')) {
      return ok(routes.chat ? routes.chat() : { user: {}, assistant: {} });
    }
    if (method === 'POST' && /\/claims\/[^/]+\/review$/.test(target)) {
      if (routes.reviewThrows) throw new TypeError('offline');
      return ok(routes.review ? routes.review() : { review: reviewRecord });
    }
    if (target.includes('/provenance')) {
      return ok(routes.provenance ? routes.provenance() : provenance);
    }
    if (target.includes('/reviews')) {
      return ok(routes.reviews ? routes.reviews() : { reviews: [] });
    }
    if (target.includes('/claims')) {
      const offset = Number(new URL(target).searchParams.get('offset') ?? '0');
      return ok(routes.claims(offset));
    }
    throw new Error(`unexpected fetch ${method} ${target}`);
  }));
  return calls;
}

afterEach(() => vi.unstubAllGlobals());

describe('ClaimHistory in a real browser', () => {
  it('renders an honest empty state', async () => {
    stubDaemon({ claims: () => claimPage([], 0, false) });
    await render(<ClaimHistory sessionId="s1" />);

    await expect.poll(bodyText).toContain('No evaluated claims');
    expect(bodyText()).not.toContain('verified');
  });

  it('renders claim evidence and requests a review without mutating the record', async () => {
    const calls = stubDaemon({ claims: () => claimPage([claim], 0, false) });
    const screen = await render(<ClaimHistory sessionId="s1" />);

    await expect.poll(bodyText).toContain('clm-test-0001');
    const text = bodyText();
    expect(text).toContain('job-1234');
    expect(text).toContain('not evaluated');

    await screen.getByRole('button', { name: 'Request review' }).click();

    await expect.poll(bodyText).toContain('Review requested');
    const ledgerCall = calls.find((call) => call.method === 'POST' && call.url.includes('/api/ledger/append'));
    expect(ledgerCall).toBeTruthy();
    expect(ledgerCall!.body).toMatchObject({ session_id: 's1', event_type: 'peer_review' });
    expect(ledgerCall!.body!.payload).toMatchObject({ claim_id: 'clm-test-0001' });

    const chatCall = calls.find((call) => call.method === 'POST' && call.url.includes('/api/chat/messages'));
    expect(chatCall).toBeTruthy();
    expect(String(chatCall!.body!.content)).toContain('clm-test-0001');
    // A review request is advisory: the recorded verdict is untouched.
    expect(bodyText()).toContain('verified');
  });

  it('records an append-only claim decision with a required reason', async () => {
    const calls = stubDaemon({ claims: () => claimPage([claim], 0, false) });
    const screen = await render(<ClaimHistory sessionId="s1" />);

    await expect.poll(bodyText).toContain('clm-test-0001');
    await screen.getByLabelText('Claim review reason').fill('uncertainty reviewed');
    await screen.getByRole('button', { name: 'accepted' }).click();

    await expect.poll(() => calls.filter((call) => call.method === 'POST' && call.url.endsWith('/review')).length).toBe(1);
    const reviewCall = calls.find((call) => call.method === 'POST' && call.url.endsWith('/review'))!;
    expect(reviewCall.url).toContain('/api/sessions/s1/claims/clm-test-0001/review');
    expect(reviewCall.body).toMatchObject({ decision: 'accepted', reason: 'uncertainty reviewed' });
    await expect.poll(bodyText).toContain('review: accepted');
  });

  it('refuses a claim decision without a reason', async () => {
    const calls = stubDaemon({ claims: () => claimPage([claim], 0, false) });
    const screen = await render(<ClaimHistory sessionId="s1" />);

    await expect.poll(bodyText).toContain('clm-test-0001');
    await screen.getByRole('button', { name: 'contested' }).click();

    await expect.poll(bodyText).toContain('review reason is required');
    expect(calls.filter((call) => call.method === 'POST' && call.url.endsWith('/review'))).toHaveLength(0);
  });

  it('shows a failed decision without pretending it was applied', async () => {
    const calls = stubDaemon({
      claims: () => claimPage([claim], 0, false),
      review: () => ({ review: null }),
    });
    const screen = await render(<ClaimHistory sessionId="s1" />);

    await expect.poll(bodyText).toContain('clm-test-0001');
    await screen.getByLabelText('Claim review reason').fill('recheck');
    await screen.getByRole('button', { name: 'reopened' }).click();

    await expect.poll(bodyText).toContain('claim review was refused');
    expect(calls.filter((call) => call.method === 'POST' && call.url.endsWith('/review'))).toHaveLength(1);
    // No decision chip is invented when the write was refused.
    expect(bodyText()).not.toContain('review: reopened');
  });

  it('loads the next newest-first page without duplicating rows', async () => {
    const older = { ...claim, id: 'clm-test-0002', statement: 'Older claim.' };
    const calls = stubDaemon({
      claims: (offset) => (offset === 0 ? claimPage([claim], 0, true) : claimPage([older], 1, false)),
    });
    const screen = await render(<ClaimHistory sessionId="s1" />);

    await expect.poll(bodyText).toContain('clm-test-0001');
    await screen.getByRole('button', { name: 'Load older claims' }).click();

    await expect.poll(bodyText).toContain('clm-test-0002');
    const text = bodyText();
    expect(text).toContain('clm-test-0001');
    expect(text.match(/clm-test-0001/g)).toHaveLength(1);
    const claimsCalls = calls.filter((call) => call.method === 'GET' && /\/claims\?/.test(call.url));
    expect(claimsCalls.some((call) => new URL(call.url).searchParams.get('offset') === '1')).toBe(true);
    expect(claimsCalls.some((call) => new URL(call.url).searchParams.get('limit') === '50')).toBe(true);
  });

  it('renders the no-session empty state without fetching', async () => {
    const fetchStub = vi.fn();
    vi.stubGlobal('fetch', fetchStub);
    await render(<ClaimHistory sessionId={null} />);

    expect(bodyText()).toContain('No evaluated claims');
    expect(fetchStub).not.toHaveBeenCalled();
  });

  it('marks a thrown decision as failed and preserves the reason', async () => {
    const calls = stubDaemon({ claims: () => claimPage([claim], 0, false), reviewThrows: true });
    const screen = await render(<ClaimHistory sessionId="s1" />);

    await expect.poll(bodyText).toContain('clm-test-0001');
    await screen.getByLabelText('Claim review reason').fill('recheck');
    await screen.getByRole('button', { name: 'accepted' }).click();

    await expect.poll(bodyText).toContain('claim review was refused');
    expect(calls.filter((call) => call.method === 'POST' && call.url.endsWith('/review'))).toHaveLength(1);
    // The reason the operator typed is preserved for a retry, not silently dropped.
    expect((screen.getByLabelText('Claim review reason').element() as HTMLInputElement).value).toBe('recheck');
  });

  it('does not invent an evidence job from malformed evidence JSON', async () => {
    stubDaemon({ claims: () => claimPage([{ ...claim, evidence_json: '{not-json' }], 0, false) });
    await render(<ClaimHistory sessionId="s1" />);

    await expect.poll(bodyText).toContain('clm-test-0001');
    expect(bodyText()).not.toContain('job job-');
  });

  it('surfaces an unavailable history instead of pretending it is empty', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => refused()));
    await render(<ClaimHistory sessionId="s1" />);

    await expect.poll(bodyText).toContain('claim history unavailable');
    expect(bodyText()).not.toContain('No evaluated claims');
  });

  it('opens typed provenance rows and an honest completeness summary', async () => {
    const calls = stubDaemon({ claims: () => claimPage([claim], 0, false) });
    const screen = await render(<ClaimHistory sessionId="s1" />);

    await expect.poll(bodyText).toContain('clm-test-0001');
    await screen.getByRole('button', { name: 'Provenance' }).click();

    await expect.poll(bodyText).toContain('Complete');
    const text = bodyText();
    expect(text).toContain('measured 1');
    expect(text).toContain('job-1234');
    expect(text).toContain('sha256:abcdef01234…');
    const provenanceCall = calls.find((call) => call.method === 'GET' && call.url.includes('/provenance'));
    expect(provenanceCall).toBeTruthy();
    expect(provenanceCall!.url).toContain('/api/sessions/s1/claims/clm-test-0001/provenance');
  });
});
