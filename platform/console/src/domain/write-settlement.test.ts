import { describe, expect, it } from 'vitest';
import { acceptWrite, IDLE_WRITE, settleWrite, writeSettlementLabel } from './write-settlement';

describe('write settlement', () => {
  it('never labels accepted as applied', () => {
    const accepted = acceptWrite('req-1', 'saving layout');
    expect(writeSettlementLabel(accepted)).toBe('accepted · saving layout');
    expect(writeSettlementLabel(settleWrite('req-1', 'applied', 'revision 4'))).toBe('applied · revision 4');
  });

  it('keeps refusal and unknown distinct', () => {
    expect(writeSettlementLabel(settleWrite('req-1', 'refused', 'revision conflict'))).toBe('refused · revision conflict');
    expect(writeSettlementLabel(settleWrite('req-1', 'unknown', 'network lost'))).toBe('unknown · network lost');
    expect(writeSettlementLabel(IDLE_WRITE)).toBe('');
  });
});
