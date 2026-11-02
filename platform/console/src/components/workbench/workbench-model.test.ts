import { describe, expect, it } from 'vitest';
import type { CampaignState } from '@/types';
import {
  buildWorkbench,
  readCamera,
  readPositions,
  readWorkbenchDocument,
  workbenchDocument,
  workbenchPayload,
} from './workbench-model';

function campaign(overrides: Partial<CampaignState> = {}): CampaignState {
  return {
    session_id: 's1',
    research_goal: 'Find the ground state',
    target_system: 'H2',
    quantum_factors: {
      target_molecule: 'H2',
      molecular_formula: 'H2',
      bond_distance: 0.7414,
      active_electrons: 2,
      active_orbitals: 2,
      fermion_mapping: 'jordan_wigner',
      qubit_count: 4,
      circuit_depth: 6,
      two_qubit_gates: 3,
      ansatz_family: 'UCCSD',
      meyer_wallach_q: 0.1,
      gradient_variance: 0.01,
      barren_plateau_risk: 'low',
      clifford_fidelity: 0.99,
      recommended_backend: 'local_simulator',
      estimated_spend_usd: 0,
      qasm: 'OPENQASM 2.0;',
      dag_nodes: [],
      dag_edges: [],
      wire_cuts: [],
    },
    classical_anchor: {
      method: 'PySCF FCI',
      rhf_mean_field_energy: -1.11,
      fci_exact_energy: -1.13727,
      vqe_expectation_energy: -1.13,
      discrepancy_mha: 7.27,
      chemical_accuracy_passed: false,
      chemical_accuracy_limit: 1.6,
      anchor_computed: true,
    },
    trajectory: [
      { step: 0, energy: -1.1, loss: 0.1, theta: [], grad_norm: 0.1, watchdog: '', timestamp: '' },
      { step: 1, energy: -1.13, loss: 0.05, theta: [], grad_norm: 0.05, watchdog: '', timestamp: '' },
    ],
    branches: [],
    epistemics: {
      alpha_corroborated: 0.8,
      beta_contested: 0.1,
      gamma_unverified: 0.1,
      p_correlated: 0.7,
      status: 'COLLAPSED_VERIFIED',
      literature_drift_mha: 0.2,
      statement: 'The ground state is within chemical accuracy.',
      sources: ['ledger:1'],
    },
    notebook: {
      id: 'n1',
      title: 'Hydrogen notebook',
      created_at: '',
      classical_anchor: 'PySCF FCI',
      quantum_energy: -1.13,
      classical_energy: -1.13727,
      delta_mha: 7.27,
      chemical_accuracy: false,
      provenance_hash: 'sha256:abc',
      epistemic_verdict: 'VERIFIED',
      sections: ['Abstract', 'Results'],
    },
    updated_at: '',
    ...overrides,
  };
}

describe('workbench model', () => {
  it('projects campaign objects without inventing an empty campaign', () => {
    expect(buildWorkbench(null)).toEqual([]);
    expect(buildWorkbench(undefined)).toEqual([]);
  });

  it('includes the campaign, measurement, circuit, and chart objects', () => {
    const objects = buildWorkbench(campaign());
    expect(objects.map((object) => object.id)).toEqual(
      expect.arrayContaining(['campaign', 'hypothesis', 'circuit', 'measurement', 'evidence', 'proof', 'manuscript', 'chart']),
    );
    const measurement = objects.find((object) => object.id === 'measurement')!;
    expect(measurement.facts.find((fact) => fact.label === 'FCI')?.value).toBe('-1.13727 Ha');
    expect(measurement.facts.find((fact) => fact.label === 'UNCERTAINTY')?.value).toBe('not recorded');
    expect(measurement.basis).toContain('not an uncertainty');
    const evidence = objects.find((object) => object.id === 'evidence')!;
    expect(evidence.sources).toEqual([{ label: 'ledger:1' }]);
    expect(evidence.basis).toContain('not a truth verdict');
    const chart = objects.find((object) => object.id === 'chart')!;
    expect(chart.chart?.series).toHaveLength(2);
    expect(chart.chart?.series[1]?.name).toBe('FCI anchor');
  });

  it('projects loop records as agent-run objects', () => {
    const objects = buildWorkbench(campaign(), [
      { id: 'loop-123456789', session_id: 's1', status: 'running', step_count: 3, max_steps: 10, diverged: false, created_at: '', updated_at: '' },
    ]);
    const run = objects.find((object) => object.kind === 'agent_run');
    expect(run?.id).toBe('agent-run-loop-123456789');
    expect(run?.facts.find((fact) => fact.label === 'Step')?.value).toBe('3 / 10');
  });

  it('projects job records as job objects', () => {
    const objects = buildWorkbench(campaign(), [], [
      { id: 'job-123456789', session_id: 's1', status: 2, job_type: 'vqe', params_json: {}, created_at: '2026-11-01T00:00:00Z', completed_at: '2026-11-01T00:00:02Z' },
    ]);
    const job = objects.find((object) => object.kind === 'job');
    expect(job?.id).toBe('job-job-123456789');
    expect(job?.facts.find((fact) => fact.label === 'Status')?.value).toBe('RUNNING');
  });

  it('projects open obligations as approval objects', () => {
    const objects = buildWorkbench(campaign(), [], [], [
      { id: 'ob-1', subject_kind: 'spend', subject_id: 'spend-123456789', owner: 'lab', disposition: 'REVIEW', required_cases: ['owner'], required_authorities: ['treasury'], close_ready: false, close_reasons: [] },
    ]);
    const approval = objects.find((object) => object.kind === 'approval');
    expect(approval?.id).toBe('approval-ob-1');
    expect(approval?.facts.find((fact) => fact.label === 'Close ready')?.value).toBe('no');
  });

  it('links only absolute HTTP(S) evidence sources and leaves references as text', () => {
    const base = campaign();
    const linked = campaign({
      epistemics: {
        ...base.epistemics!,
        sources: ['https://example.org/paper', 'ledger:42', 'not a url'],
      },
    });
    const evidence = buildWorkbench(linked).find((object) => object.id === 'evidence')!;
    expect(evidence.sources).toEqual([
      { label: 'https://example.org/paper', url: 'https://example.org/paper' },
      { label: 'ledger:42' },
      { label: 'not a url' },
    ]);
  });

  it('labels an uncomputed anchor as UNVERIFIED', () => {
    const unanchored = campaign({
      classical_anchor: {
        method: 'uncomputed',
        rhf_mean_field_energy: 0,
        fci_exact_energy: 0,
        vqe_expectation_energy: 0,
        discrepancy_mha: 0,
        chemical_accuracy_passed: false,
        chemical_accuracy_limit: 1.6,
        anchor_computed: false,
      },
      trajectory: [],
    });
    const objects = buildWorkbench(unanchored);
    const measurement = objects.find((object) => object.id === 'measurement')!;
    expect(measurement.title).toBe('No measured anchor');
    expect(measurement.facts.find((fact) => fact.label === 'FCI')?.value).toBe('UNVERIFIED');
    expect(objects.some((object) => object.id === 'chart')).toBe(false);
  });

  it('round-trips a bounded camera and rejects malformed layouts', () => {
    expect(readCamera(null)).toEqual({ x: 36, y: 36, zoom: 0.82 });
    expect(readCamera({ workbench: { camera: { x: 10, y: 20, zoom: 1.5 } } })).toEqual({
      x: 10,
      y: 20,
      zoom: 1.5,
    });
    expect(readCamera({ workbench: { camera: { x: 'nope', y: 20, zoom: 1 } } })).toEqual({
      x: 36,
      y: 36,
      zoom: 0.82,
    });
    expect(workbenchPayload({ x: 10.1234, y: 20.9876, zoom: 1.23456 }, { campaign: { x: 1.234, y: -2.345 } })).toEqual({
      version: 1,
      camera: { x: 10.12, y: 20.99, zoom: 1.235 },
      objects: { campaign: { x: 1.23, y: -2.35 } },
    });
    expect(readPositions({ workbench: { objects: { campaign: { x: 4, y: 5 }, bad: { x: 'nope', y: 1 } } } })).toEqual({
      campaign: { x: 4, y: 5 },
    });
    expect(
      workbenchPayload({ x: 0, y: 0, zoom: 1 }, { campaign: { x: 4, y: 5 } }, { campaign: { width: 380, height: 200 } }),
    ).toEqual({
      version: 1,
      camera: { x: 0, y: 0, zoom: 1 },
      objects: { campaign: { x: 4, y: 5, width: 380, height: 200 } },
    });
  });

  it('round-trips the v2 document without duplicating content', () => {
    const objects = buildWorkbench(campaign());
    const doc = workbenchDocument('s1', objects, {}, {}, { x: 10, y: 20, zoom: 1.1 });
    const read = readWorkbenchDocument(doc as unknown as Record<string, unknown>);
    expect(read.camera).toEqual({ x: 10, y: 20, zoom: 1.1 });
    expect(read.positions.campaign).toEqual({ x: 0, y: 0 });
    expect(doc.objects[0].ref).toEqual({ domain: 'campaign', id: 's1' });
    expect('title' in doc.objects[0]).toBe(false);
    expect(doc.links).toEqual(expect.arrayContaining([{ id: 'campaign-hypothesis', source: 'campaign', target: 'hypothesis', kind: 'supports' }]));
    expect(read.links).toHaveLength(doc.links.length);
    const withHidden = workbenchDocument('s1', objects, {}, {}, { x: 0, y: 0, zoom: 1 }, undefined, undefined, {}, ['campaign']);
    expect(readWorkbenchDocument(withHidden as unknown as Record<string, unknown>).hidden).toEqual(['campaign']);
  });
});
