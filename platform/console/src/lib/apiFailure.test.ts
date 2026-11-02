import { describe, expect, it } from 'vitest';
import { ApiFailureError, OperationError, apiFailureFrom, failedResult } from './api-errors';

describe('API failure classification', () => {
  it('classifies auth, forbidden, server, transport, and contract failures', () => {
    expect(apiFailureFrom(new OperationError('x', 401, 'no', '/api/loops')).kind).toBe('auth');
    expect(apiFailureFrom(new OperationError('x', 403, 'no', '/api/x')).kind).toBe('forbidden');
    expect(apiFailureFrom(new OperationError('x', 503, 'down', '/api/x')).kind).toBe('server');
    expect(apiFailureFrom(new OperationError('x', null, 'offline', '/api/x')).kind).toBe('transport');
    expect(apiFailureFrom(new Error('response contract drifted')).kind).toBe('contract');
  });

  it('preserves the operation path and error message in a failed result', () => {
    const result = failedResult(new OperationError('fetchLoops', 401, 'login required', '/api/loops'), '/fallback');
    expect(result.ok).toBe(false);
    expect(result.error).toBe('login required');
    expect(result.failure).toMatchObject({ path: '/api/loops', status: 401, kind: 'auth' });
  });

  it('uses an existing failure without rebuilding it', () => {
    const failure = { path: '/api/jobs', status: 500, kind: 'server' as const, message: 'failed', at: 1 };
    expect(apiFailureFrom(new ApiFailureError(failure))).toEqual(failure);
  });
});
