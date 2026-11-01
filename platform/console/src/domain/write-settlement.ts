export type WritePhase = 'idle' | 'accepted' | 'applied' | 'refused' | 'unknown';

export type WriteSettlement = {
  phase: WritePhase;
  requestId: string | null;
  detail: string;
};

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
