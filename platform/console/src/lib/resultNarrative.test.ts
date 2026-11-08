import { describe, expect, it } from 'vitest';
import { numericClaimGuard, numericClaims, unsupportedNumericClaims } from './resultNarrative';

describe('numeric claim guard', () => {
  it('accepts numbers carried by the result record', () => {
    const text = 'Measured -1.137783 Ha ± 1.384 mHa; Δ 0.513 mHa against FCI -1.137270 Ha.';
    expect(numericClaims(text)).toEqual(['-1.137783', '1.384', '0.513', '-1.137270']);
    expect(unsupportedNumericClaims(text, [-1.1377829714, 1.3835463, 0.5127968, -1.1372701747])).toEqual([]);
  });

  it('rejects a number absent from the result record', () => {
    const guard = numericClaimGuard('The measured energy is 8.901 Ha.', [-1.1377829714]);
    expect(guard.ok).toBe(false);
    expect(guard.unsupported).toContain('8.901');
  });

  it('rejects a fabricated precision that does not match the recorded value', () => {
    expect(unsupportedNumericClaims('energy 9.2 Ha', [9.1])).toEqual(['9.2']);
  });
});
