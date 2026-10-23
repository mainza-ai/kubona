import { describe, expect, it } from 'vitest';
import { buildPinPayload, pinValuesFromHit } from './pin';

describe('probe-pin helpers (R-24)', () => {
  const energy = (dR: number, z: number) => dR * dR + z;

  it('derives r/energy from a hit with display rounding', () => {
    const v = pinValuesFromHit(1.0, 0.5, 1.5, energy);
    // deltaR = 0.4 -> r = 1.9, energy = 0.16 + 0.5 = 0.66
    expect(v.r).toBe(1.9);
    expect(v.energy).toBe(0.66);
  });

  it('rounds r to 4dp and energy to 5dp', () => {
    const v = pinValuesFromHit(0.123456, 0.987654, 1.0, energy);
    const dR = 0.123456 * 0.4;
    expect(v.r).toBe(Number((1.0 + dR).toFixed(4)));
    expect(v.energy).toBe(Number((dR * dR + 0.987654).toFixed(5)));
  });

  it('every payload is a human touch: touched + actor are unconditional', () => {
    const p = buildPinPayload({ r: 1.9, energy: 0.66 });
    expect(p.world_tool).toBe('probe_pin');
    expect(p.touched).toBe(true);
    expect(p.actor).toBe('human');
    expect(p.bond_distance).toBe(1.9);
    expect(p.energy).toBe(0.66);
  });

  it('origin hit yields the equilibrium point, still touched', () => {
    const p = buildPinPayload(pinValuesFromHit(0, 0, 2.0, energy));
    expect(p.bond_distance).toBe(2.0);
    expect(p.energy).toBe(0);
    expect(p.touched).toBe(true);
  });
});
