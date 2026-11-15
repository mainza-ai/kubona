import { afterEach, describe, expect, it, vi } from 'vitest';
import { deleteSession } from './api';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('deleteSession refusal transport', () => {
  it('preserves blockers, code, and status from a typed 409', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      code: 'delete_blocked',
      message: 'session s not deleted: [live loops] (settle or abort first)',
      details: { blockers: ['live loops', 'outstanding spend reservations'] },
    }), { status: 409, headers: { 'Content-Type': 'application/json' } })));
    const result = await deleteSession('s');
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.code).toBe('delete_blocked');
    expect(result.status).toBe(409);
    expect(result.blockers).toEqual(['live loops', 'outstanding spend reservations']);
    expect(result.error).toContain('live loops');
  });

  it('reports a committed deletion with a failed audit receipt as success-with-warning', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      deleted: true,
      session_id: 's',
      receipt: 'failed',
      receipt_error: 'ledger append failed',
    }), { status: 200, headers: { 'Content-Type': 'application/json' } })));
    const result = await deleteSession('s');
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.receiptFailed).toContain('ledger append failed');
  });
});
