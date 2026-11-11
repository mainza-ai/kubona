import { describe, expect, it } from 'vitest';
import type { CampaignState } from '@/types';
import {
  AUTO_CLUSTER_THRESHOLD,
  MAX_WORKBENCH_ZOOM,
  MIN_WORKBENCH_ZOOM,
  alignSelectedPositions,
  arrangeSelectedPositions,
  boardPin,
  buildDefaultClusterAreas,
  buildWorkbench,
  buildWorkbenchLinks,
  cameraShowsAnyObject,
  computeFitCamera,
  distributeSelectedPositions,
  dropSpot,
  factKeyOverlapRatio,
  filterWorkbenchObjects,
  freeSpot,
  getNodeRenderSize,
  hasExplicitCameraRecord,
  nextZoomTier,
  readCamera,
  readPositions,
  readWorkbenchDocument,
  readWorkbenchView,
  reflowUnpinnedObjects,
  resolveFocusObjectId,
  routeOrthogonalBusEdges,
  selectFocusedLinks,
  totalOverlapArea,
  workbenchDocument,
  workbenchPayload,
  workbenchViewDocument,
  zoomCameraAtPoint,
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

  it('surfaces measurement uncertainty and the below-FCI excursion truthfully', () => {
    const measured = campaign({
      measurement: {
        job_id: 'job-1', claim_id: 'claim-1', method: 'VQE', ansatz: 'hardware-efficient',
        backend: 'qiskit-aer-simulator', energy: -1.1377829714, energy_exact: -1.1372070030,
        fci_exact_energy: -1.1372701747, uncertainty_mha: 1.3835463, delta_mha: 0.5127968,
        shots: 16384, seed: 7, chemical_accuracy: true, converged: false, measured: true,
        below_fci: true, source: 'shot-sampled Pauli expectation',
      },
      pes_points: [
        { bond_distance: 0.7, energy: -1.1371356, fci_exact_energy: -1.1361895, delta_mha: 0.946, uncertainty_mha: 1.867, chemical_accuracy: true },
        { bond_distance: 0.9, energy: -1.1212512, fci_exact_energy: -1.1205603, delta_mha: 0.691, uncertainty_mha: 1.996, chemical_accuracy: true },
      ],
    });
    const objects = buildWorkbench(measured);
    const measurement = objects.find((object) => object.id === 'measurement')!;
    expect(measurement.facts.find((fact) => fact.label === 'Uncertainty')?.value).toBe('± 1.384 mHa');
    expect(measurement.facts.find((fact) => fact.label === 'Converged')?.value).toBe('no (evaluation cap)');
    expect(measurement.facts.find((fact) => fact.label === 'Admissibility')?.value).toContain('POINT BELOW FCI');
    const chart = objects.find((object) => object.id === 'pes-chart')!;
    expect(chart.chart?.series[0]?.name).toBe('VQE energy');
    expect(chart.chart?.series[0]?.points.map((p) => p.x)).toEqual([0.7, 0.9]);
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

  it('round-trips the v2 semantic layout document without camera/selection or unpinned coords', () => {
    const objects = buildWorkbench(campaign());
    expect(totalOverlapArea(objects)).toBe(0);
    const links = buildWorkbenchLinks(objects);

    const doc = workbenchDocument('s1', objects, { campaign: { x: 16, y: 24 } }, {}, { campaign: true }, links);
    expect('camera' in (doc as Record<string, unknown>)).toBe(false);
    expect('selected' in (doc as Record<string, unknown>)).toBe(false);
    expect(doc.objects[0]).toEqual(
      expect.objectContaining({ id: 'campaign', position: { x: 16, y: 24 }, pinned: true }),
    );

    const read = readWorkbenchDocument(doc as unknown as Record<string, unknown>);
    expect(read.pins).toEqual({ campaign: true });
    expect(read.positions.campaign).toEqual({ x: 16, y: 24 });
    expect(doc.objects[0].ref).toEqual({ domain: 'campaign', id: 's1' });
    expect('title' in doc.objects[0]).toBe(false);
    expect(doc.links).toEqual(expect.arrayContaining([{ id: 'campaign-hypothesis', source: 'campaign', target: 'hypothesis', kind: 'supports' }]));
    expect(read.links).toHaveLength(doc.links.length);

    const withHidden = workbenchDocument('s1', objects, {}, {}, {}, [], undefined, {}, ['campaign']);
    expect(readWorkbenchDocument(withHidden as unknown as Record<string, unknown>).hidden).toEqual(['campaign']);
  });

  it('round-trips the ephemeral view document and rescues legacy layout cameras', () => {
    const view = workbenchViewDocument({
      camera: { x: 10, y: 20, zoom: 1.1 },
      selected: ['campaign'],
      view_mode: 'list',
    });
    expect(readWorkbenchView(view as unknown as Record<string, unknown>)).toEqual({
      camera: { x: 10, y: 20, zoom: 1.1 },
      selected: ['campaign'],
      view_mode: 'list',
      filters: {},
      panel: '',
      lod: 'compact',
    });
    expect(['summary', 'compact', 'detail']).toContain(view.lod);
    expect(typeof view.panel).toBe('string');
    const legacy = readWorkbenchView(null, { camera: { x: 12, y: 24, zoom: 0.9 } });
    expect(legacy.camera).toEqual({ x: 12, y: 24, zoom: 0.9 });
  });

  it('resolves non-overlapping 8px-snapped placements and drop spots', () => {
    const objects = buildWorkbench(campaign());
    const obstacles = objects.map((o) => ({ x: o.position.x, y: o.position.y, width: o.size.width, height: o.size.height }));
    const spot = freeSpot({ width: 356, height: 180 }, { x: 0, y: 0 }, obstacles);
    expect(Math.abs(spot.x % 8)).toBe(0);
    expect(Math.abs(spot.y % 8)).toBe(0);
    expect(totalOverlapArea([...objects, { ...objects[0], id: 'extra', position: spot, size: { width: 356, height: 180 } }])).toBe(0);

    const settled = dropSpot({ x: 384, y: 0, width: 356, height: 220 }, obstacles);
    expect(Math.abs(settled.x % 8)).toBe(0);
    expect(Math.abs(settled.y % 8)).toBe(0);
    expect(boardPin(1, 1)).toBe(false);
    expect(boardPin(8, 0)).toBe(true);
  });

  it('folds repeatable jobs onto root card with transient history cap and respects pins', () => {
    const base = campaign();
    const repeatJobs = [
      { id: 'td-1', session_id: 's1', status: 'COMPLETED', job_type: 'thermodynamic_limit', params_json: { target: 'H2' }, result_json: { energy: -1.1 }, created_at: '2026-11-01T00:00:00Z' },
      { id: 'td-2', session_id: 's1', status: 'COMPLETED', job_type: 'thermodynamic_limit', params_json: { target: 'H2' }, result_json: { energy: -1.11 }, created_at: '2026-11-01T00:01:00Z' },
      { id: 'td-3', session_id: 's1', status: 'COMPLETED', job_type: 'thermodynamic_limit', params_json: { target: 'H2' }, result_json: { energy: -1.12 }, created_at: '2026-11-01T00:02:00Z' },
      { id: 'td-4', session_id: 's1', status: 'COMPLETED', job_type: 'thermodynamic_limit', params_json: { target: 'H2' }, result_json: { energy: -1.13 }, created_at: '2026-11-01T00:03:00Z', completed_at: '2026-11-01T00:03:10Z' },
    ];
    const collapsed = buildWorkbench(base, [], repeatJobs);
    const jobs = collapsed.filter((o) => o.kind === 'job');
    expect(jobs).toHaveLength(1);
    expect(jobs[0].id).toBe('job-td-1');
    expect(jobs[0].facts.find((f) => f.label === 'Runs')?.value).toBe('4 (3 folded)');
    expect(jobs[0].facts.find((f) => f.label === 'Delta')?.value).toBe('-0.03000 Ha');
    expect(jobs[0].detail).toContain('+1 older runs');

    const withPin = buildWorkbench(base, [], repeatJobs, [], [], undefined, { 'job-td-4': true });
    expect(withPin.filter((o) => o.kind === 'job')).toHaveLength(1);
    expect(resolveFocusObjectId('td-4', withPin)).toBe('job-td-1');

    // Heterogeneous param conventions ({}, { target: 'H2' }, { campaign_id: 's1' }) fold into one canonical group,
    // while distinct molecules ({ molecule: 'LiH' }) remain distinct.
    const mixedRuns = [
      { id: 'cr-1', session_id: 's1', status: 'COMPLETED', job_type: 'circuit_run', params_json: {}, result_json: { energy: -1.1 }, created_at: '2026-11-01T00:00:00Z' },
      { id: 'cr-2', session_id: 's1', status: 'COMPLETED', job_type: 'circuit_run', params_json: { target: 'H2' }, result_json: { energy: -1.12 }, created_at: '2026-11-01T00:01:00Z' },
      { id: 'cr-3', session_id: 's1', status: 'COMPLETED', job_type: 'circuit_run', params_json: { campaign_id: 's1' }, result_json: { energy: -1.13 }, created_at: '2026-11-01T00:02:00Z' },
      { id: 'cr-4', session_id: 's1', status: 'COMPLETED', job_type: 'circuit_run', params_json: { molecule: 'LiH' }, result_json: { energy: -7.88 }, created_at: '2026-11-01T00:03:00Z' },
    ];
    const mixedObjects = buildWorkbench(base, [], mixedRuns);
    expect(mixedObjects.filter((o) => o.kind === 'job')).toHaveLength(2);
    expect(resolveFocusObjectId('cr-3', mixedObjects)).toBe('job-cr-1');
    expect(resolveFocusObjectId('cr-4', mixedObjects)).toBe('job-cr-4');
  });

  it('deduplicates Hypothesis, Evidence, and Table cards without losing truth-verdict or source contracts', () => {
    const sentinel = campaign({
      epistemics: {
        alpha_corroborated: -1,
        beta_contested: -1,
        gamma_unverified: -1,
        p_correlated: -1,
        status: 'CONTESTED',
        literature_drift_mha: -1,
        statement: 'VQE measured -1.115 Ha vs FCI -1.137 Ha.',
        sources: ['bridge:vqe_repeats:6183b5db-e351-4657-b07c-3e99e2523bd5'],
      },
    });
    const objects = buildWorkbench(sentinel);
    const hyp = objects.find((o) => o.id === 'hypothesis')!;
    const ev = objects.find((o) => o.id === 'evidence')!;
    const table = objects.find((o) => o.id === 'quantum-factors')!;

    expect(hyp.facts.some((f) => f.label === 'Amplitudes (α/β/γ)')).toBe(true);
    expect(hyp.facts.filter((f) => f.value === 'not evaluated')).toHaveLength(0);
    expect(hyp.meta?.callout?.summary).toContain('Tribunal amplitudes pending');
    expect(ev.facts.some((f) => /Corroborated α|Contested β|Unverified γ/i.test(f.label))).toBe(false);
    expect(ev.title).toContain('Attribution ledger');
    expect(ev.basis).toContain('not a truth verdict');
    expect(table.facts.some((f) => f.label === 'Mapping' || f.label === 'Bond distance')).toBe(false);
    expect(table.meta?.comparison?.rows.some((r) => r.label === 'RHF mean-field')).toBe(true);
  });

  it('fuses >=50% overlapping chart factors, folds comparison tables, and weaves evidence into claim prose', () => {
    const base = campaign();
    const objects = buildWorkbench(base, [], [
      {
        id: 'cmp-1',
        session_id: 's1',
        status: 'COMPLETED',
        job_type: 'comparison',
        params_json: { holder: 'measurement' },
        result_json: {
          kind: 'comparison',
          comparison: {
            columns: ['Ansatz', 'Energy', 'Δ'],
            rows: [{ label: 'UCCSD', values: ['-1.137 Ha', '0.5 mHa'] }],
          },
          factors: { Steps: 2, Initial: '-1.10000 Ha', Optimizer: 'COBYLA' },
        },
        created_at: '2026-11-01T00:00:00Z',
      },
    ]);
    expect(objects.some((o) => o.id === 'job-cmp-1')).toBe(false);
    const measurement = objects.find((o) => o.id === 'measurement')!;
    expect(measurement.meta?.comparison?.rows).toHaveLength(1);
    expect(measurement.facts.find((f) => f.label === 'Comparison')?.value).toBe('1 rows folded');

    const chart = objects.find((o) => o.id === 'chart')!;
    expect(chart.facts.find((f) => f.label === 'Optimizer')?.value).toBe('COBYLA');
    expect(factKeyOverlapRatio([{ label: 'Steps' }, { label: 'Initial' }], [{ label: 'Steps' }, { label: 'Latest' }])).toBe(0.5);

    const hypothesis = objects.find((o) => o.id === 'hypothesis')!;
    expect(hypothesis.detail).toContain('ledger:1');
    expect(hypothesis.sources).toEqual([{ label: 'ledger:1' }]);
  });

  it('builds foreign-key links, caps focused relations at 6, and routes orthogonal elbows', () => {
    const objects = buildWorkbench(
      campaign(),
      [{ id: 'l1', session_id: 's1', status: 'running', step_count: 1, max_steps: 5, diverged: false, created_at: '', updated_at: '' }],
      [{ id: 'j1', session_id: 's1', status: 2, job_type: 'vqe', params_json: {}, created_at: '' }],
      [{ id: 'ob1', subject_kind: 'job', subject_id: 'j1', owner: 'lab', disposition: 'open', required_cases: [], required_authorities: [], close_ready: false, close_reasons: [] }],
      [{ id: 't1', session_id: 's1', title: 'Check j1', status: 'open', source_domain: 'job', source_id: 'j1', created_at: '', updated_at: '' }],
    );
    const links = buildWorkbenchLinks(objects);
    expect(links.some((l) => l.source === 'campaign' && l.target === 'agent-run-l1')).toBe(true);
    expect(links.some((l) => l.source === 'job-j1' && l.target === 'approval-ob1')).toBe(true);
    expect(links.some((l) => l.source === 'job-j1' && l.target === 'task-t1')).toBe(true);

    const focused = selectFocusedLinks(links, ['campaign', 'measurement']);
    expect(focused.length).toBeLessThanOrEqual(6);

    const byId = new Map(objects.map((o) => [o.id, o]));
    const routed = routeOrthogonalBusEdges(focused, byId, new Set());
    expect(routed.length).toBe(focused.length);
    for (const edge of routed) {
      expect(edge.d).toContain('M');
      expect(edge.d).not.toContain(' C');
    }

    const dragRouted = routeOrthogonalBusEdges(focused, byId, new Set(['campaign']));
    expect(dragRouted.length).toBe(focused.length);
  });

  it('aligns, distributes, and arranges selected objects onto 8px grid with zero overlap', () => {
    const objects = buildWorkbench(campaign()).slice(0, 4);
    const aligned = alignSelectedPositions(objects, 'left');
    const distributed = distributeSelectedPositions(objects);
    const arranged = arrangeSelectedPositions(objects);
    for (const map of [aligned, distributed, arranged]) {
      expect(Object.keys(map)).toHaveLength(objects.length);
      for (const pos of Object.values(map)) {
        expect(Math.abs(pos.x % 8)).toBe(0);
        expect(Math.abs(pos.y % 8)).toBe(0);
      }
      const moved = objects.map((o) => ({ ...o, position: map[o.id] ?? o.position }));
      expect(totalOverlapArea(moved)).toBe(0);
    }
  });

  it('applies zoom-tier hysteresis deadbands, filters subsets, and resolves folded job IDs', () => {
    expect(nextZoomTier(0.45, 'standard')).toBe('overview');
    expect(nextZoomTier(0.55, 'overview')).toBe('overview'); // hysteresis hold below 0.60
    expect(nextZoomTier(0.62, 'overview')).toBe('standard');
    expect(nextZoomTier(1.08, 'standard')).toBe('detail');
    expect(nextZoomTier(0.98, 'detail')).toBe('detail'); // hysteresis hold above 0.95
    expect(nextZoomTier(0.92, 'detail')).toBe('standard');

    const objects = buildWorkbench(
      campaign({
        epistemics: {
          alpha_corroborated: 0.2,
          beta_contested: 0.7,
          gamma_unverified: 0.1,
          p_correlated: 0.2,
          status: 'CONTESTED',
          literature_drift_mha: 4.5,
          statement: 'Contested energy',
          sources: ['ledger:1'],
        },
      }),
      [{ id: 'l1', session_id: 's1', status: 'running', step_count: 1, max_steps: 5, diverged: false, created_at: '', updated_at: '' }],
      [
        { id: 'td-1', session_id: 's1', status: 'COMPLETED', job_type: 'thermodynamic_limit', params_json: { target: 'H2' }, result_json: { energy: -1.12 }, created_at: '2026-11-01T00:00:00Z' },
        { id: 'td-2', session_id: 's1', status: 'COMPLETED', job_type: 'thermodynamic_limit', params_json: { target: 'H2' }, result_json: { energy: -1.13 }, created_at: '2026-11-01T00:01:00Z' },
      ],
    );
    expect(filterWorkbenchObjects(objects, 'spine').some((o) => o.kind === 'campaign')).toBe(true);
    expect(filterWorkbenchObjects(objects, 'jobs').every((o) => o.kind === 'job' || o.kind === 'agent_run')).toBe(true);
    expect(filterWorkbenchObjects(objects, 'issues').some((o) => /CONTESTED/i.test(`${o.title} ${o.meta?.status ?? ''}`))).toBe(true);

    // Both folded and root job IDs resolve to the owning root card (job-td-1).
    expect(resolveFocusObjectId('td-1', objects)).toBe('job-td-1');
    expect(resolveFocusObjectId('td-2', objects)).toBe('job-td-1');
  });
});


describe('energyChart outlier framing', () => {
  it('preserves raw outlier points in series while clamping y axis to target band', () => {
    const outlierCampaign = campaign({
      classical_anchor: {
        method: 'PySCF STO-3G Full Configuration Interaction',
        rhf_mean_field_energy: -1.11668,
        fci_exact_energy: -1.13727,
        vqe_expectation_energy: -1.136,
        discrepancy_mha: 1.27,
        chemical_accuracy_limit: 1.6,
        chemical_accuracy_passed: true,
        anchor_computed: true,
      },
      trajectory: [
        { step: 1, energy: -1.129, loss: 0.008, theta: [0.1], grad_norm: 0.1, watchdog: 'measured_vqe', timestamp: '2026-11-11T00:00:00Z', source: 'measured' },
        { step: 2, energy: -7.85, loss: 6.71, theta: [0.2], grad_norm: 0.2, watchdog: 'measured_vqe', timestamp: '2026-11-11T00:01:00Z', source: 'measured' },
        { step: 3, energy: -1.136, loss: 0.001, theta: [0.3], grad_norm: 0.01, watchdog: 'measured_vqe', timestamp: '2026-11-11T00:02:00Z', source: 'measured' },
      ],
    });
    const objects = buildWorkbench(outlierCampaign);
    const chartObj = objects.find((object) => object.id === 'chart');
    expect(chartObj?.chart).toBeDefined();
    const spec = chartObj!.chart!;
    expect(spec.series[0]?.points.map((p) => p.y)).toEqual([-1.129, -7.85, -1.136]);
    expect(spec.y?.clamp).toBe(true);
    expect(spec.y?.min).toBeGreaterThan(-1.2);
    expect(spec.y?.max).toBeLessThan(-1.0);
    expect(spec.basis).toContain('1 off-scale step flagged');
  });
});

describe('getNodeRenderSize, overview reflow, and auto-clustering', () => {
  it('accounts for OpenQASM code height and avoids overlap across standard and overview tiers', () => {
    const qasm = Array.from({ length: 14 }, (_, i) => `rz(0.${i}) q[${i % 2}];`).join('\n');
    const base = campaign();
    const c = campaign({ quantum_factors: { ...base.quantum_factors!, qasm } });
    const objects = buildWorkbench(c);
    const circuit = objects.find((object) => object.id === 'circuit');
    expect(circuit).toBeDefined();
    expect(circuit!.size.height).toBeGreaterThan(280);
    expect(totalOverlapArea(objects)).toBe(0);

    const overviewSized = objects.map((object) => ({
      ...object,
      size: getNodeRenderSize(object, 'overview'),
    }));
    const overviewLayout = reflowUnpinnedObjects(overviewSized, {}, {}, 20, 'overview', {});
    expect(totalOverlapArea(overviewLayout)).toBe(0);
    const overviewCircuit = overviewLayout.find((object) => object.id === 'circuit');
    expect(overviewCircuit!.size.height).toBeLessThan(circuit!.size.height);
  });

  it('builds a Jobs & Runs auto-cluster area only above AUTO_CLUSTER_THRESHOLD', () => {
    const small = buildWorkbench(campaign());
    expect(buildDefaultClusterAreas(small).areas).toHaveLength(0);

    const manyJobs = Array.from({ length: AUTO_CLUSTER_THRESHOLD }, (_, i) => ({
      id: `vqe-${i}`,
      session_id: 's1',
      status: 'COMPLETED',
      job_type: 'vqe',
      params_json: { step: i },
      result_json: { energy: -1.1 },
      created_at: '2026-11-11T00:00:00Z',
    }));
    const large = buildWorkbench(campaign(), [], manyJobs);
    expect(large.length).toBeGreaterThan(AUTO_CLUSTER_THRESHOLD);
    const cluster = buildDefaultClusterAreas(large);
    expect(cluster.areas).toHaveLength(1);
    expect(cluster.areas[0]?.id).toBe('auto-jobs-runs');
    expect(cluster.areas[0]?.title).toBe('Jobs & Runs');
    expect(Object.keys(cluster.assignments).length).toBe(AUTO_CLUSTER_THRESHOLD);
  });
});

describe('computeFitCamera, cameraShowsAnyObject, and zoomCameraAtPoint', () => {
  it('detects explicit camera records across view and workbench documents', () => {
    expect(hasExplicitCameraRecord(null)).toBe(false);
    expect(hasExplicitCameraRecord({})).toBe(false);
    expect(hasExplicitCameraRecord({ camera: { x: 'bad', y: 0, zoom: 1 } })).toBe(false);
    expect(hasExplicitCameraRecord({ camera: { x: 10, y: -20, zoom: 0.75 } })).toBe(true);
    expect(hasExplicitCameraRecord({ workbench: { camera: { x: 0, y: 0, zoom: 1 } } })).toBe(true);
  });

  it('reports false when a camera strands all cards outside the visible viewport', () => {
    const objs = [
      { position: { x: -208, y: -176 }, size: { width: 356, height: 220 } },
      { position: { x: 440, y: 120 }, size: { width: 356, height: 260 } },
    ];
    const viewport = { width: 960, height: 520 };
    expect(cameraShowsAnyObject({ x: -3000, y: -3000, zoom: 0.82 }, viewport, objs)).toBe(false);
    expect(cameraShowsAnyObject({ x: 96.72, y: -698.72, zoom: 0.607 }, viewport, objs)).toBe(false);
    expect(cameraShowsAnyObject({ x: 250, y: 200, zoom: 0.82 }, viewport, objs)).toBe(true);
    expect(cameraShowsAnyObject({ x: -3000, y: -3000, zoom: 1 }, viewport, [])).toBe(true);
    expect(cameraShowsAnyObject({ x: -3000, y: -3000, zoom: 1 }, { width: 0, height: 0 }, objs)).toBe(true);
  });

  it('anchors top/left cards inside viewport padding even when tall graphs hit min zoom', () => {
    const box = { x: -208, y: -272, width: 1992, height: 2600 };
    const viewport = { width: 1024, height: 520 };
    const fitClosed = computeFitCamera(box, viewport, { minimapOpen: false });
    expect(fitClosed.zoom).toBe(MIN_WORKBENCH_ZOOM);
    expect(fitClosed.zoom).toBeLessThanOrEqual(MAX_WORKBENCH_ZOOM);
    // Top edge of topmost card (`box.y * zoom + y`) must stay >= padTop (28px).
    expect(box.y * fitClosed.zoom + fitClosed.y).toBeGreaterThanOrEqual(27);
    expect(box.x * fitClosed.zoom + fitClosed.x).toBeGreaterThanOrEqual(35);

    const fitCompact = computeFitCamera({ x: 40, y: 40, width: 800, height: 320 }, viewport);
    expect(fitCompact.zoom).toBeGreaterThan(0.8);
    expect(fitCompact.zoom).toBeLessThanOrEqual(1.15);

    const fitReadableSpine = computeFitCamera({ x: 40, y: 40, width: 1500, height: 740 }, { width: 1200, height: 420 }, { minZoom: 0.68 });
    expect(fitReadableSpine.zoom).toBe(0.68);
    expect(40 * fitReadableSpine.zoom + fitReadableSpine.y).toBeGreaterThanOrEqual(27);
  });

  it('zooms around the focal point while staying inside server zoom bounds', () => {
    const focal = { x: 480, y: 300 };
    const start = { x: 100, y: 40, zoom: 0.8 };
    const worldBeforeX = (focal.x - start.x) / start.zoom;
    const worldBeforeY = (focal.y - start.y) / start.zoom;

    const zoomedIn = zoomCameraAtPoint(start, 1.6, focal);
    expect(zoomedIn.zoom).toBe(1.6);
    expect((focal.x - zoomedIn.x) / zoomedIn.zoom).toBeCloseTo(worldBeforeX, 1);
    expect((focal.y - zoomedIn.y) / zoomedIn.zoom).toBeCloseTo(worldBeforeY, 1);

    expect(zoomCameraAtPoint(start, 9, focal).zoom).toBe(MAX_WORKBENCH_ZOOM);
    expect(zoomCameraAtPoint(start, 0.05, focal).zoom).toBe(MIN_WORKBENCH_ZOOM);
  });
});
