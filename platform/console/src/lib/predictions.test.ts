import { beforeEach, describe, expect, it, vi } from 'vitest';
import { evaluatePrediction, fetchLatestPrediction, sealPrediction } from './api';

// P3 contract pins: the console posts ledger-shaped payloads and surfaces
// server-computed surprise (never invents z/Brier client-side).

function mockFetchOnce(body: unknown, ok = true, status = 200) {
  (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
    ok,
    status,
    json: async () => body,
  });
}

describe('vault pre-registration client', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  it('seal posts the full commitment and returns the hash', async () => {
    mockFetchOnce({ ok: true, prediction_hash: 'ab'.repeat(32), expected_std_used: 0.005, std_floor_applied: false });
    const res = await sealPrediction({
      sessionId: 'sess-1',
      targetSystem: 'H2',
      method: 'VQE UCCSD + Qiskit Aer',
      quantity: 'ground_state_energy_ha',
      expectedMean: -1.137,
      expectedStd: 0.005,
    });
    expect(res.ok).toBe(true);
    expect(res.prediction_hash).toHaveLength(64);
    const [, init] = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit];
    const body = JSON.parse(init.body as string);
    expect(body.session_id).toBe('sess-1');
    expect(body.expected_mean).toBe(-1.137);
    expect(body.p_correlated).toBe(0.88);
    // Authorization rides authHeaders() (env-dependent console token);
    // the no-token shape is asserted here, the required-token shape Go-side.
    expect(init.headers).toMatchObject({ 'Content-Type': 'application/json' });
  });

  it('evaluate posts hash + measured value and surfaces server math', async () => {
    mockFetchOnce({ ok: true, surprise_z: 0.16, surprise_ratio: 0.16, brier_score: 7.3e-6, within_3sigma: true, level: 'normal' });
    const res = await evaluatePrediction({ sessionId: 'sess-1', predictionHash: 'ab'.repeat(32), measuredValue: -1.1362 });
    expect(res.ok).toBe(true);
    expect(res.surprise_z).toBeCloseTo(0.16, 9);
    expect(res.level).toBe('normal');
  });

  it('latest parses sealed + evaluation records', async () => {
    mockFetchOnce({
      ok: true,
      sealed: { prediction_hash: 'ab'.repeat(32), expected_mean: -1.137, expected_std: 0.005 },
      evaluation: { prediction_hash: 'ab'.repeat(32), surprise_z: 0.16, level: 'normal' },
    });
    const res = await fetchLatestPrediction('sess-1');
    expect(res.ok).toBe(true);
    expect(res.sealed?.expected_mean).toBe(-1.137);
    expect(res.evaluation?.level).toBe('normal');
  });

  it('refusals surface the backend message', async () => {
    mockFetchOnce({ message: 'expected_std must be positive' }, false, 400);
    const res = await sealPrediction({
      sessionId: 'sess-1',
      targetSystem: 'H2',
      method: 'VQE',
      quantity: 'ground_state_energy_ha',
      expectedMean: -1.137,
      expectedStd: 0,
    });
    expect(res.ok).toBe(false);
    expect(res.error).toContain('expected_std must be positive');
  });
});
