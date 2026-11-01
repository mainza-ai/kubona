'use client';

import { API_BASE, apiFetch, authHeaders } from '@/lib/api';
import { API_OPERATIONS, type ApiOperationName, type ApiOperationResponse } from './openapi';

export class OperationError extends Error {
  constructor(
    readonly operation: string,
    readonly status: number | null,
    message: string,
  ) {
    super(message);
    this.name = 'OperationError';
  }
}

type RequestOptions = {
  params?: Record<string, string>;
  query?: Record<string, string | number | boolean | undefined>;
  body?: unknown;
  headers?: Record<string, string>;
};

/**
 * Typed request for a modeled operation. The generated path/method/response
 * mapping is the contract; callers supply only parameters and body.
 */
export async function requestOperation<K extends ApiOperationName>(
  name: K,
  { params = {}, query = {}, body, headers }: RequestOptions = {},
): Promise<ApiOperationResponse[K]> {
  const operation = API_OPERATIONS[name];
  const path = operation.path.replace(/\{([^}]+)\}/g, (_, key: string) => {
    const value = params[key];
    if (!value) throw new OperationError(name, null, `missing path parameter ${key}`);
    return encodeURIComponent(value);
  });
  const qs = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined) qs.set(key, String(value));
  }
  const url = `${API_BASE}${path}${qs.toString() ? `?${qs.toString()}` : ''}`;
  const res = await apiFetch(url, {
    method: operation.method.toUpperCase(),
    cache: 'no-store',
    headers: authHeaders(body === undefined ? headers : { 'Content-Type': 'application/json', ...headers }),
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  if (!res.ok) {
    throw new OperationError(name, res.status, `${operation.method.toUpperCase()} ${operation.path} refused (HTTP ${res.status})`);
  }
  return (await res.json()) as ApiOperationResponse[K];
}
