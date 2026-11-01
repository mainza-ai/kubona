'use client';

import { useCallback, useState } from 'react';
import {
  acceptWrite,
  newWriteRequestId,
  settleWrite,
  IDLE_WRITE,
  type WriteSettlement,
} from './write-settlement';

/** Shared write lifecycle for pages that need retained request identity. */
export function useWriteSettlement() {
  const [write, setWrite] = useState<WriteSettlement>(IDLE_WRITE);

  const begin = useCallback((detail: string) => {
    const requestId = newWriteRequestId();
    setWrite(acceptWrite(requestId, detail));
    return requestId;
  }, []);

  const applied = useCallback((requestId: string, detail: string) => {
    setWrite(settleWrite(requestId, 'applied', detail));
  }, []);

  const refused = useCallback((requestId: string, detail: string) => {
    setWrite(settleWrite(requestId, 'refused', detail));
  }, []);

  const unknown = useCallback((requestId: string, detail: string) => {
    setWrite(settleWrite(requestId, 'unknown', detail));
  }, []);

  return { write, begin, applied, refused, unknown };
}
