import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { QuantumSettings } from './QuantumSettings';
import * as api from '@/lib/api';

vi.mock('@/lib/api', () => ({
  fetchQuantumBackends: vi.fn(),
  fetchQuantumSettings: vi.fn(),
  saveQuantumSettings: vi.fn(),
  runQuantumCanary: vi.fn(),
}));

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

async function flush() {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

const BACKENDS = [
  { provider: 'local-simulator', display_name: 'Local simulator', modality: 'simulator', device: 'local', credential_env: '', credential_state: 'ready', telemetry: 'measured', executable_now: true },
  { provider: 'ibm-qiskit', display_name: 'IBM Quantum', modality: 'superconducting', device: 'ibm_heron', credential_env: 'KUBONA_IBM_QISKIT_TOKEN', credential_state: 'credentialless', telemetry: 'declared', executable_now: false },
  { provider: 'neutral-atom-quera-pasqal', display_name: 'Neutral atom (not wired)', modality: 'neutral_atom', device: 'unwired', credential_env: '', credential_state: 'unavailable', telemetry: 'declared', executable_now: false },
];

describe('QuantumSettings provider selection', () => {
  beforeEach(() => {
    vi.mocked(api.fetchQuantumBackends).mockResolvedValue(BACKENDS);
    vi.mocked(api.fetchQuantumSettings).mockResolvedValue({ default_backend: 'local-simulator', harness_backends: {} });
    vi.mocked(api.saveQuantumSettings).mockResolvedValue({ ok: true });
  });

  it('lists providers with credential and catalog honesty, never a key field', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<QuantumSettings />);
    });
    const text = textOf(root!.root);
    expect(text).toContain('IBM Quantum');
    expect(text).toContain('NO CREDENTIALS');
    expect(text).toContain('CATALOG');
    // Secrets are never accepted here: no key/password input exists,
    // and the footer states the rule.
    const inputs = root!.root.findAll(
      (n) => n.type === 'input' && (n.props as { type?: string }).type === 'password',
    );
    expect(inputs).toHaveLength(0);
    expect(text).toContain('never accepted here');
  });

  it('saves the selected default through the validated route', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<QuantumSettings />);
    });
    const radios = root!.root.findAll(
      (n) => n.type === 'input' && (n.props as { type?: string }).type === 'radio',
    );
    expect(radios.length).toBeGreaterThan(0);
    // Select IBM (credentialless-by-design is allowed: plan before token).
    await act(async () => {
      (radios[1].props as { onChange: () => void }).onChange();
    });
    const saveBtn = root!.root.find(
      (n) => n.type === 'button' && textOf(n).includes('Save selection'),
    );
    await act(async () => {
      await (saveBtn.props as { onClick: () => Promise<void> }).onClick();
    });
    expect(api.saveQuantumSettings).toHaveBeenCalledWith({
      default_backend: 'ibm-qiskit',
      harness_backends: {},
    });
    expect(textOf(root!.root)).toContain('saved');
  });

  it('surfaces backend refusals verbatim', async () => {
    vi.mocked(api.saveQuantumSettings).mockResolvedValue({ ok: false, error: 'unknown default_backend "nope"' });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<QuantumSettings />);
    });
    // Make dirty (select IBM), then save fails → refusal renders verbatim.
    const radios = root!.root.findAll(
      (n) => n.type === 'input' && (n.props as { type?: string }).type === 'radio',
    );
    await act(async () => {
      (radios[1].props as { onChange: () => void }).onChange();
    });
    const saveBtn = root!.root.find(
      (n) => n.type === 'button' && textOf(n).includes('Save selection'),
    );
    await act(async () => {
      await (saveBtn.props as { onClick: () => Promise<void> }).onClick();
    });
    expect(textOf(root!.root)).toContain('unknown default_backend');
  });

  it('runs the canary and reports honestly', async () => {
    vi.mocked(api.runQuantumCanary).mockResolvedValue({ ok: true, results: { 'local-simulator': {} }, cached: false });
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<QuantumSettings />);
    });
    await flush();
    const buttons = root!.root.findAll((n) => n.type === 'button');
    const canaryBtn = buttons.find((b) => String((b.props as Record<string, unknown>).title ?? '').includes('canary'));
    expect(canaryBtn).toBeDefined();
    await act(async () => {
      (canaryBtn!.props as { onClick: () => void }).onClick();
    });
    await flush();
    expect(api.runQuantumCanary).toHaveBeenCalled();
    expect(textOf(root!.root)).toContain('canary ran');
  });
});
