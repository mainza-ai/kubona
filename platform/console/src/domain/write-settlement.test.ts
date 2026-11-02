import { afterEach, describe, expect, it, vi } from 'vitest';
import { acceptWrite, IDLE_WRITE, reconcileOperation, settleWrite, writeSettlementLabel } from './write-settlement';

afterEach(() => vi.unstubAllGlobals());

describe('write settlement', () => {
  it('never labels accepted as applied', () => {
    const accepted = acceptWrite('req-1', 'saving layout');
    expect(writeSettlementLabel(accepted)).toBe('accepted · saving layout');
    expect(writeSettlementLabel(settleWrite('req-1', 'applied', 'revision 4'))).toBe('applied · revision 4');
  });

  it('reconciles an unknown write from its server receipt', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 404, json: async () => ({}) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({
        request_id: 'req-2', status: 'complete', status_code: 200,
        receipt: { status: 'saved', revision: 2 },
      }) }));
    const settled = await reconcileOperation('req-2', 's1', 2, 0);
    expect(writeSettlementLabel(settled)).toBe('applied · saved');
  });

  it('keeps refusal and unknown distinct', () => {
    expect(writeSettlementLabel(settleWrite('req-1', 'refused', 'revision conflict'))).toBe('refused · revision conflict');
    expect(writeSettlementLabel(settleWrite('req-1', 'unknown', 'network lost'))).toBe('unknown · network lost');
    expect(writeSettlementLabel(IDLE_WRITE)).toBe('');
  });
});
