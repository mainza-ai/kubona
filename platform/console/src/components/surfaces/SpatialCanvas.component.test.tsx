import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { SpatialCanvas } from './SpatialCanvas';
import type { CampaignState } from '@/types';

// Migrated from react-test-renderer (SpatialCanvas.test.tsx). The old suite
// walked `tree.toJSON()` with a textOf() helper, so it read props and text
// nodes that were never painted; two of its assertions could only ever see the
// lazy fallback because react-test-renderer never resolves a dynamic import.
// These assertions read the live DOM in Chromium: the workbench chrome, the
// depth switch, the ADMET cells, and the lazy boundary (skeleton → real 3D
// observatory) are all observed where the operator sees them.
//
// No `vi.mock('@/lib/api')`: the canvas/ADMET/observatory reads are answered
// at the fetch boundary, so the wrapper, the operation contract and the query
// strings the console really sends stay under test.

type Call = { method: string; path: string; url: string };

const ADMET_LIH = {
  smiles: '[Li+].[H-]',
  molecular_weight: 7.949,
  clogp: -2.88,
  tpsa: 0.0,
  synthesizability: 'High (Synthesizable)',
  lipinski: { pass: true, violations: 0 },
};

const ADMET_NO_SYNTH = {
  smiles: '[H][H]',
  molecular_weight: 2.016,
  clogp: 0.0,
  tpsa: 0.0,
  // no synthesizability key: missing must render as missing.
};

function ok(payload: unknown) {
  return { ok: true, status: 200, json: async () => payload };
}

/** Answers the canvas/ADMET/observatory reads; records every call. */
function installCanvasFetch(admet: unknown | ((smiles: string) => unknown) = ADMET_LIH) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const parsed = new URL(String(url));
    const call: Call = { method: (init?.method ?? 'GET').toUpperCase(), path: parsed.pathname, url: String(url) };
    calls.push(call);
    if (call.method === 'GET' && call.path === '/api/canvas/layout') return ok({});
    if (call.method === 'PUT' && call.path === '/api/canvas/layout') return ok({ ok: true });
    if (call.method === 'GET' && call.path === '/api/chemistry/admet') {
      const smiles = parsed.searchParams.get('smiles') ?? '';
      return ok(typeof admet === 'function' ? (admet as (s: string) => unknown)(smiles) : admet);
    }
    if (call.method === 'GET' && call.path === '/api/observatory/lens/history') {
      return ok({ ok: true, operations: [] });
    }
    if (call.method === 'GET' && call.path === '/api/vault/predictions/latest') {
      return ok({ ok: true, sealed: null, evaluation: null, assertion: null });
    }
    if (call.method === 'GET' && call.path === '/api/vault/predictions/calibration') {
      return ok({ ok: true, authors: {}, evaluations: 0 });
    }
    if (call.method === 'GET' && call.path === '/api/vault/predictions/history') {
      return ok({ ok: true, count: 0, predictions: [] });
    }
    if (call.method === 'POST' && call.path === '/api/ledger/export') return ok({ ok: true, events: [] });
    return {
      ok: false,
      status: 404,
      json: async () => ({ error: `unexpected ${call.method} ${call.path}` }),
      text: async () => `unexpected ${call.method} ${call.path}`,
    };
  });
  return calls;
}

function text(screen: { container: HTMLElement }): string {
  return screen.container.textContent ?? '';
}

function campaign(partial: Record<string, unknown>): CampaignState {
  return partial as unknown as CampaignState;
}

const H2_ANCHORED = {
  target_system: 'H2',
  classical_anchor: { anchor_computed: true, fci_exact_energy: -1.137 },
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('SpatialCanvas Depth 1 in a real browser', () => {
  it('renders physical molecular system with bond distance and ADMET profile in Depth 1', async () => {
    const calls = installCanvasFetch(ADMET_LIH);
    const screen = await render(
      <SpatialCanvas
        campaign={campaign({
          target_system: 'LiH',
          quantum_factors: {
            target_molecule: 'LiH',
            molecular_formula: 'LiH',
            bond_distance: 1.5957,
          },
          classical_anchor: {
            anchor_computed: true,
            fci_exact_energy: -7.882,
            rhf_mean_field_energy: -7.863,
          },
        })}
      />,
    );

    expect(text(screen)).toContain('Lithium Hydride (LiH)');
    expect(text(screen)).toContain('Nuclear Core');
    expect(text(screen)).toContain('Molecular Pharmacology & ADMET');

    // The profile is the bridge's answer for this molecule, not a constant.
    const admetCall = calls.find((c) => c.path === '/api/chemistry/admet');
    expect(admetCall).toBeDefined();
    expect(new URL(admetCall!.url).searchParams.get('smiles')).toBe('[Li+].[H-]');
    await expect.poll(() => text(screen)).toContain('7.95');
    expect(text(screen)).toContain('Da');
    expect(text(screen)).toContain('High');
  });

  it('renders missing synthesizability as missing, never as a flattering grade (H2)', async () => {
    installCanvasFetch(ADMET_NO_SYNTH);
    const screen = await render(<SpatialCanvas campaign={campaign(H2_ANCHORED)} />);

    // The synthesis cell carries an empty title exactly when the value is
    // missing (?? ''), so this pins the missing-renders-missing behavior.
    await expect.poll(() => text(screen)).toContain('Synthesis');
    const missing = [...screen.container.querySelectorAll('div[title]')].find(
      (el) => (el.textContent ?? '') === '—',
    );
    expect(missing).toBeDefined();
    expect(missing!.getAttribute('title')).toBe('');
    expect(text(screen)).not.toContain('High');
  });

  it('tags inferred SMILES and table equilibrium (H8, H9)', async () => {
    installCanvasFetch(ADMET_LIH);
    // No campaign SMILES and no bond distance: both fallbacks engage.
    const screen = await render(<SpatialCanvas campaign={campaign(H2_ANCHORED)} />);

    expect(text(screen)).toContain('(inferred)');
    expect(text(screen)).toContain('(table)');
    await screen.unmount();

    // Campaign-supplied values: no fallback tags.
    const live = await render(
      <SpatialCanvas
        campaign={campaign({
          ...H2_ANCHORED,
          smiles: '[H][H]',
          quantum_factors: { target_molecule: 'H2', molecular_formula: 'H2', bond_distance: 0.7414 },
        })}
      />,
    );
    expect(text(live)).not.toContain('(inferred)');
    expect(text(live)).not.toContain('(table)');
  });
});

describe('SpatialCanvas progressive disclosure in a real browser', () => {
  it('switches Depth 2 (Quantum Circuit) and Depth 3 (Integrals) by real clicks', async () => {
    installCanvasFetch(ADMET_LIH);
    const screen = await render(<SpatialCanvas campaign={campaign(H2_ANCHORED)} />);

    const depth2 = screen.getByRole('button', { name: '2. Quantum Circuit' });
    await expect.element(depth2).toBeInTheDocument();
    await depth2.click();

    expect(text(screen)).toContain('QUBITS');
    expect(text(screen)).toContain('OpenQASM');

    const depth3 = screen.getByRole('button', { name: '3. Exact Integrals' });
    await expect.element(depth3).toBeInTheDocument();
    await depth3.click();

    expect(text(screen)).toContain('Full Configuration Interaction (FCI) Benchmark');
    expect(text(screen)).toContain('Second-Quantized Hamiltonian Mapping');
    // Depth 2 content is gone: one depth is shown at a time.
    expect(text(screen)).not.toContain('QUBITS');
  });

  it('does not synthesize a schematic QASM when the campaign has no circuit', async () => {
    installCanvasFetch(ADMET_LIH);
    const screen = await render(<SpatialCanvas campaign={campaign(H2_ANCHORED)} />);

    await screen.getByRole('button', { name: '2. Quantum Circuit' }).click();

    expect(text(screen)).toContain('No circuit proposal is recorded');
    expect(text(screen)).not.toContain('schematic — illustrative');
    // The QASM pane refuses in the same words instead of inventing a circuit.
    await screen.getByRole('button', { name: 'OpenQASM' }).click();
    expect(text(screen)).toContain('No circuit proposal is recorded for this campaign.');
  });
});

describe('SpatialCanvas representation modes in a real browser', () => {
  // Wave-3 code-split. This is the FIRST observatory entry in this page, so
  // the SpatialCanvas3D chunk is still cold and the lazy boundary has to paint
  // its fallback. Every later observatory entry in this file reuses the loaded
  // chunk: React then commits the scene without ever painting a fallback, so
  // those tests assert the settled state instead of a transient the operator
  // would only see on a cold load.
  it('observatory fallback is an honest skeleton, never blank', async () => {
    installCanvasFetch(ADMET_LIH);
    const screen = await render(<SpatialCanvas campaign={campaign(H2_ANCHORED)} />);

    await screen.getByRole('button', { name: '3D Observatory' }).click();

    // The panel is labeled and says what is happening, never an empty box.
    const skeleton = screen.container.querySelector('[aria-label="Loading 3D observatory"]');
    expect(skeleton).not.toBeNull();
    expect(skeleton!.getAttribute('role')).toBe('status');
    expect(skeleton!.textContent).toContain('Loading 3D observatory…');
    // And it is a passage, not a resting state.
    await expect.poll(() => text(screen), { timeout: 15000 }).toContain('Observatory controls');
  });

  it('workbench first paint never mounts the 3D fallback', async () => {
    installCanvasFetch(ADMET_LIH);
    const screen = await render(<SpatialCanvas campaign={campaign(H2_ANCHORED)} />);

    // three.js stays unloaded: no observatory skeleton on first paint.
    expect(text(screen)).not.toContain('Loading 3D observatory');
    expect(text(screen)).not.toContain('Observatory controls');
    expect(screen.container.querySelector('[aria-label="Loading 3D observatory"]')).toBeNull();
  });

  it('switches between 2D Workbench and 3D Observatory representation mode', async () => {
    installCanvasFetch(ADMET_LIH);
    const screen = await render(
      <SpatialCanvas campaign={campaign({ ...H2_ANCHORED, epistemics: { p_correlated: 0.92 } })} />,
    );

    // 2D Workbench is initially active: its depth switcher is on screen and
    // the observatory branch has not been mounted.
    const workbench = screen.getByRole('button', { name: '2D Workbench' });
    const observatory = screen.getByRole('button', { name: '3D Observatory' });
    await expect.element(workbench).toHaveAttribute('aria-pressed', 'true');
    await expect.element(observatory).toHaveAttribute('aria-pressed', 'false');
    expect(text(screen)).toContain('1. Physical System');
    expect(text(screen)).not.toContain('Loading 3D observatory');

    await observatory.click();
    await expect.element(observatory).toHaveAttribute('aria-pressed', 'true');
    await expect.element(workbench).toHaveAttribute('aria-pressed', 'false');
    // The workbench-only depth switcher is gone with the workbench.
    expect(text(screen)).not.toContain('1. Physical System');

    // The split resolves into the real observatory, never a dead end.
    await expect.poll(() => text(screen), { timeout: 15000 }).toContain('Observatory controls');
    expect(text(screen)).not.toContain('Loading 3D observatory');
  });

  it('renders the Vault pre-registration panel in Observatory mode', async () => {
    const calls = installCanvasFetch(ADMET_LIH);
    const screen = await render(
      <SpatialCanvas
        campaign={campaign({
          session_id: 'sess-vault-test',
          ...H2_ANCHORED,
          epistemics: { p_correlated: 0.92 },
        })}
      />,
    );

    await screen.getByRole('button', { name: '3D Observatory' }).click();

    // The old suite could only reach the lazy skeleton here; in a browser the
    // split resolves and the panel that lives in SpatialCanvas3D is on screen.
    await expect.poll(() => text(screen), { timeout: 15000 }).toContain('Vault Pre-registration');
    expect(text(screen)).toContain('No sealed prediction — ghost is hidden.');
    const latest = calls.find((c) => c.path === '/api/vault/predictions/latest');
    expect(latest).toBeDefined();
    expect(new URL(latest!.url).searchParams.get('session_id')).toBe('sess-vault-test');
  });
});
