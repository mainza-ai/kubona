import { API_BASE, apiFetch, authHeaders } from '@/lib/api';

export type WritePhase = 'idle' | 'accepted' | 'applied' | 'refused' | 'unknown';

export type WriteSettlement = {
  phase: WritePhase;
  requestId: string | null;
  detail: string;
};

export function newWriteRequestId(): string {
  return globalThis.crypto?.randomUUID?.() ?? `write-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export const IDLE_WRITE: WriteSettlement = { phase: 'idle', requestId: null, detail: '' };

export function acceptWrite(requestId: string, detail = 'request accepted'): WriteSettlement {
  return { phase: 'accepted', requestId, detail };
}

export function settleWrite(
  requestId: string,
  phase: Exclude<WritePhase, 'idle' | 'accepted'>,
  detail: string,
): WriteSettlement {
  return { phase, requestId, detail };
}

/** Accepted is not applied; unknown never renders as success. */
export function writeSettlementLabel(write: WriteSettlement): string {
  switch (write.phase) {
    case 'accepted':
      return `accepted · ${write.detail}`;
    case 'applied':
      return `applied · ${write.detail}`;
    case 'refused':
      return `refused · ${write.detail}`;
    case 'unknown':
      return `unknown · ${write.detail}`;
    default:
      return '';
  }
}


type OperationReceipt = {
  request_id?: string;
  status?: 'in_progress' | 'complete';
  status_code?: number;
  receipt?: unknown;
};

async function readOperationReceipt(requestId: string, sessionId: string): Promise<OperationReceipt | null> {
  const path = `/api/operations/${encodeURIComponent(requestId)}?session_id=${encodeURIComponent(sessionId)}`;
  const res = await apiFetch(`${API_BASE}${path}`, { cache: 'no-store', headers: authHeaders() });
  if (res.status === 404) return null;
  if (!res.ok) throw new Error(`operation receipt refused (HTTP ${res.status})`);
  return (await res.json()) as OperationReceipt;
}

/** Reconcile an accepted-but-unconfirmed write against its server receipt. */
export async function reconcileOperation(
  requestId: string,
  sessionId: string,
  attempts = 8,
  delayMs = 250,
): Promise<WriteSettlement> {
  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    try {
      const receipt = await readOperationReceipt(requestId, sessionId);
      if (receipt?.status === 'complete') {
        const code = receipt.status_code ?? 0;
        const detail = typeof receipt.receipt === 'string'
          ? receipt.receipt
          : receipt.receipt && typeof receipt.receipt === 'object'
            ? String((receipt.receipt as { message?: unknown; error?: unknown; status?: unknown }).message
              ?? (receipt.receipt as { error?: unknown }).error
              ?? (receipt.receipt as { status?: unknown }).status
              ?? `HTTP ${code}`)
            : `HTTP ${code}`;
        return settleWrite(requestId, code >= 400 ? 'refused' : 'applied', detail.slice(0, 200));
      }
    } catch {
      // Keep checking; a transient receipt read is not an outcome.
    }
    if (attempt < attempts) await new Promise((resolve) => setTimeout(resolve, delayMs));
  }
  return settleWrite(requestId, 'unknown', `operation ${requestId.slice(0, 12)} not confirmed after reconciliation`);
}
