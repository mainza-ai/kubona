import { describe, expect, it } from 'vitest';
import { API_OPERATIONS } from './openapi';

describe('generated OpenAPI contract', () => {
  it('maps modeled operations to one path and response', () => {
    expect(API_OPERATIONS.fetchSessionsResult).toEqual({
      method: 'get',
      path: '/api/sessions',
      response: 'sessions.list',
    });
    expect(API_OPERATIONS.fetchFleet.path).toBe('/api/fleet');
  });
});
