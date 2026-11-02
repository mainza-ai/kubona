export class OperationError extends Error {
  constructor(
    readonly operation: string,
    readonly status: number | null,
    message: string,
    readonly path: string | null = null,
  ) {
    super(message);
    this.name = 'OperationError';
  }
}

export type ApiFailureKind = 'transport' | 'auth' | 'forbidden' | 'refusal' | 'server' | 'contract';

export type ApiFailure = {
  path: string;
  status: number | null;
  kind: ApiFailureKind;
  message: string;
  at: number;
};

export class ApiFailureError extends Error {
  constructor(readonly failure: ApiFailure) {
    super(failure.message);
    this.name = 'ApiFailureError';
  }
}

export function apiFailureFrom(error: unknown, path = 'api'): ApiFailure {
  const at = Date.now();
  if (error instanceof ApiFailureError) return error.failure;
  if (error instanceof OperationError) {
    const status = error.status;
    const kind: ApiFailureKind =
      status === 401 ? 'auth'
        : status === 403 ? 'forbidden'
          : status === null ? 'transport'
            : status >= 500 ? 'server'
              : 'refusal';
    return { path: error.path ?? path, status, kind, message: error.message, at };
  }
  const message = error instanceof Error ? error.message : String(error);
  return {
    path,
    status: null,
    kind: /contract/i.test(message) ? 'contract' : 'transport',
    message,
    at,
  };
}

export function failedResult(error: unknown, path: string): { ok: false; error: string; failure: ApiFailure } {
  const failure = apiFailureFrom(error, path);
  return { ok: false, error: failure.message, failure };
}
