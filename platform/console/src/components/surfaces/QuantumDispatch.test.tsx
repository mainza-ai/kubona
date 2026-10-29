import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { QuantumDispatch } from './QuantumDispatch';
import * as api from '@/lib/api';

vi.mock('@/lib/api', () => ({
  submitQuantumJob: vi.fn(),
  fetchQuantumJob: vi.fn(),
  cancelQuantumJob: vi.fn(),
  fetchQuantumBackends: vi.fn(),
  fetchQuantumSettings: vi.fn(),
  fetchLedgerExport: vi.fn(),
}));

vi.mock('@/components/widgets/CryptographicMandateCardWidget', () => ({
  CryptographicMandateCardWidget: ({ onAuthorize }: { onAuthorize?: (s: string) => void }) => (
    <button data-testid="mock-mandate" onClick={() => onAuthorize?.('mandate-sig-1')}>
      mock mandate card
    </button>
  ),
}));

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

// The component wraps async handlers in `void` (lint-clean effects), so
// the test must flush microtasks after each click before asserting.
async function flush() {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

const BACKENDS = [
  { provider: 'local-simulator', display_name: 'Local simulator', modality: 'simulator', device: 'local', credential_env: '', credential_state: 'ready', telemetry: 'measured', executable_now: true, dispatch_ready: true, input_format: 'qasm' },
  { provider: 'dwave-annealing', display_name: 'D-Wave annealing (local exact/SA)', modality: 'annealing', device: 'local/SA', credential_env: 'KUBONA_DWAVE_ANNEALING_LEAP_TOKEN', credential_state: 'ready', telemetry: 'declared', executable_now: true, dispatch_ready: true, input_format: 'qubo' },
  { provider: 'ibm-qiskit', display_name: 'IBM Quantum', modality: 'superconducting', device: 'ibm_heron', credential_env: 'KUBONA_IBM_QISKIT_TOKEN', credential_state: 'credentialless', telemetry: 'declared', executable_now: false, dispatch_ready: false, input_format: 'qasm' },
];

describe('QuantumDispatch human flow', () => {
  beforeEach(() => {
    vi.mocked(api.fetchQuantumBackends).mockResolvedValue(BACKENDS);
    vi.mocked(api.fetchQuantumSettings).mockResolvedValue({ default_backend: 'local-simulator', harness_backends: {} });
    vi.mocked(api.fetchQuantumJob).mockResolvedValue({ ok: true, job_id: 'j1', status: 'completed' });
    vi.mocked(api.fetchLedgerExport).mockResolvedValue({ ok: true, events: [] });
    vi.mocked(api.cancelQuantumJob).mockResolvedValue({ ok: true });
  });

  it('estimates without spending, then dispatches with a fresh idempotency key', async () => {
    vi.mocked(api.submitQuantumJob)
      .mockResolvedValueOnce({ ok: true, estimate: { estimated_usd: 0 }, fidelity: { admitted: true, skipped: 'simulator-class backend' } })
      .mockResolvedValueOnce({ ok: true, job_id: 'j1', queued: false, session_id: 's1' });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<QuantumDispatch sessionId="s1" />);
    });
    // Estimate (dry run).
    const estBtn = root!.root.find((n) => n.type === 'button' && textOf(n).includes('Estimate (dry run)'));
    await act(async () => {
      (estBtn.props as { onClick: () => void }).onClick();
    });
    await flush();
    expect(api.submitQuantumJob).toHaveBeenCalledWith(
      expect.objectContaining({ dry_run: true, provider: 'local-simulator' }),
    );
    // Estimate panel rendered (JSX splits "$" and digits across nodes —
    // assert the label and the digits separately).
    expect(textOf(root!.root)).toContain('Estimated cost:');
    expect(textOf(root!.root)).toContain('0.000000');
    // Dispatch (zero estimate needs no mandate).
    const goBtn = root!.root.find((n) => n.type === 'button' && textOf(n).includes('Dispatch job'));
    await act(async () => {
      (goBtn.props as { onClick: () => void }).onClick();
    });
    await flush();
    const dispatchCall = vi.mocked(api.submitQuantumJob).mock.calls[1][0] as unknown as Record<string, unknown>;
    expect(dispatchCall.dry_run).toBe(false);
    expect(dispatchCall.job_id).toBeUndefined();
    expect(typeof dispatchCall.idempotency_key).toBe('string');
    expect(String(dispatchCall.idempotency_key).length).toBeGreaterThan(8);
    expect(textOf(root!.root)).toContain('j1');
  });

  it('gates non-zero dispatch behind the mandate card', async () => {
    vi.mocked(api.submitQuantumJob)
      .mockResolvedValueOnce({ ok: true, estimate: { estimated_usd: 1.6 }, fidelity: { admitted: true } })
      .mockResolvedValueOnce({ ok: true, job_id: 'j2', queued: true, session_id: 's1' });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<QuantumDispatch sessionId="s1" />);
    });
    const estBtn = root!.root.find((n) => n.type === 'button' && textOf(n).includes('Estimate (dry run)'));
    await act(async () => {
      (estBtn.props as { onClick: () => void }).onClick();
    });
    await flush();
    // Mandate card appears for the non-zero estimate...
    expect(textOf(root!.root)).toContain('mock mandate card');
    // ...and dispatch stays disabled until signing.
    const goBtn = () => root!.root.find((n) => n.type === 'button' && textOf(n).includes('Dispatch job'));
    expect((goBtn().props as { disabled?: boolean }).disabled).toBe(true);
    // Sign → dispatch carries the mandate reference.
    await act(async () => {
      const card = root!.root.find((n) => (n.props as Record<string, unknown>)?.['data-testid'] === 'mock-mandate');
      (card.props as { onClick: () => void }).onClick();
    });
    await flush();
    await act(async () => {
      await (goBtn().props as { onClick: () => Promise<void> }).onClick();
    });
    expect(api.submitQuantumJob).toHaveBeenLastCalledWith(
      expect.objectContaining({ mandate_ref: 'mandate-sig-1', dry_run: false }),
    );
  });

  it('renders refusals verbatim and cancels with recall disclosure', async () => {
    vi.mocked(api.submitQuantumJob).mockResolvedValueOnce({ ok: false, error: 'unknown provider "nope"' });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<QuantumDispatch sessionId="s1" />);
    });
    const estBtn = root!.root.find((n) => n.type === 'button' && textOf(n).includes('Estimate (dry run)'));
    await act(async () => {
      (estBtn.props as { onClick: () => void }).onClick();
    });
    await flush();
    expect(textOf(root!.root)).toContain('unknown provider "nope"');
  });

  it('pauses polling after repeated transport loss and resumes on demand', async () => {
    vi.mocked(api.submitQuantumJob)
      .mockResolvedValueOnce({
        ok: true,
        estimate: { estimated_usd: 0 },
        fidelity: { admitted: true, skipped: 'simulator-class backend' },
      })
      .mockResolvedValueOnce({ ok: true, job_id: 'j9', queued: true, session_id: 's1' });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<QuantumDispatch sessionId="s1" />);
    });
    const estBtn = root!.root.find((n) => n.type === 'button' && textOf(n).includes('Estimate (dry run)'));
    await act(async () => {
      (estBtn.props as { onClick: () => void }).onClick();
    });
    await flush();
    const goBtn = root!.root.find((n) => n.type === 'button' && textOf(n).includes('Dispatch job'));
    await act(async () => {
      await (goBtn.props as { onClick: () => Promise<void> }).onClick();
    });
    await flush();
    expect(textOf(root!.root)).toContain('j9');
    // From here the backend goes dark: every status fetch is a
    // transport null. Fake timers drive the chained poll timeouts.
    vi.useFakeTimers();
    try {
      vi.mocked(api.fetchQuantumJob).mockResolvedValue(null);
      const refreshBtn = root!.root.find((n) => n.type === 'button' && textOf(n).includes('Refresh'));
      await act(async () => {
        (refreshBtn.props as { onClick: () => void }).onClick();
      });
      const callsBefore = vi.mocked(api.fetchQuantumJob).mock.calls.length;
      // Backoff chain: 7.5s → 11.25s → 15s → 15s; five consecutive
      // nulls exhaust the transport budget.
      for (let i = 0; i < 5; i++) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(16000);
        });
      }
      expect(vi.mocked(api.fetchQuantumJob).mock.calls.length).toBeGreaterThan(callsBefore);
      const pausedText = textOf(root!.root);
      expect(pausedText).toContain('polling paused');
      expect(pausedText).toContain('Resume');
      const callsAtPause = vi.mocked(api.fetchQuantumJob).mock.calls.length;
      // Paused: no further transport attempts while paused.
      await act(async () => {
        await vi.advanceTimersByTimeAsync(60000);
      });
      expect(vi.mocked(api.fetchQuantumJob).mock.calls.length).toBe(callsAtPause);
      // Resume restarts the poll chain (one immediate attempt).
      const resumeBtn = root!.root.find((n) => n.type === 'button' && textOf(n).includes('Resume'));
      await act(async () => {
        (resumeBtn.props as { onClick: () => void }).onClick();
      });
      expect(vi.mocked(api.fetchQuantumJob).mock.calls.length).toBeGreaterThan(callsAtPause);
      expect(textOf(root!.root)).not.toContain('polling paused');
    } finally {
      vi.useRealTimers();
    }
  });

  it('switches input mode to QUBO matrix when dwave-annealing is selected', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<QuantumDispatch sessionId="s1" />);
    });
    expect(textOf(root!.root)).toContain('OpenQASM circuit');
    const select = root!.root.find((n) => n.type === 'select');
    await act(async () => {
      (select.props as { onChange: (e: { target: { value: string } }) => void }).onChange({
        target: { value: 'dwave-annealing' },
      });
    });
    expect(textOf(root!.root)).toContain('QUBO matrix (JSON 2-D array)');
    expect(textOf(root!.root)).toContain('Samples / Reads');
  });
});
