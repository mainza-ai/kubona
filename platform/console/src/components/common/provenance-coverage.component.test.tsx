import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import {
  MolecularActiveSpaceWidget,
  QuantumCircuitQIREditorWidget,
  EpistemicManifoldSurfaceWidget,
} from '@/components/widgets';
import { buildWorkbench } from '@/components/workbench/workbench-model';
import type { CampaignState } from '@/types';

// Migrated from react-test-renderer (provenance-coverage.test.tsx): the
// invariant is a DOM one — every [data-metric] node must sit inside a
// [data-provenance] subtree — so it is asserted against the real rendered
// tree, where closest() is the browser's own ancestor walk rather than a
// hand-rolled props walk over test-renderer instances.
function assertMetricsCarryProvenance(container: HTMLElement) {
  const metrics = Array.from(container.querySelectorAll('[data-metric]'));
  expect(metrics.length).toBeGreaterThan(0);
  for (const metric of metrics) {
    const scope = metric.closest('[data-provenance]');
    expect(scope, `metric without provenance: ${metric.textContent ?? ''}`).not.toBeNull();
  }
}

const campaign = {
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
    qubit_count: 2,
    circuit_depth: 4,
    two_qubit_gates: 1,
    ansatz_family: 'UCCSD',
    recommended_backend: 'local_simulator',
    estimated_spend_usd: 0,
    qasm: 'OPENQASM 2.0;',
    dag_nodes: [],
    dag_edges: [],
    wire_cuts: [],
  },
  classical_anchor: {
    method: 'PySCF FCI',
    rhf_mean_field_energy: -1.116,
    fci_exact_energy: -1.137,
    vqe_expectation_energy: -1.136,
    discrepancy_mha: 1.0,
    chemical_accuracy_passed: true,
    chemical_accuracy_limit: 1.6,
    anchor_computed: true,
  },
  trajectory: [],
  branches: [],
  epistemics: null,
  notebook: null,
  updated_at: '',
} as unknown as CampaignState;

describe('provenance coverage in a real browser', () => {
  it('tags molecular proposal metrics with proposed provenance', async () => {
    const screen = await render(
      <MolecularActiveSpaceWidget moleculeName="H2" numElectrons={2} numOrbitals={2} homoLumoGapHa={0.05} />,
    );
    expect(screen.container.querySelector('[data-provenance="proposed"]')).not.toBeNull();
    assertMetricsCarryProvenance(screen.container);
  });

  it('tags QIR receipt metrics with measured provenance', async () => {
    const screen = await render(
      <QuantumCircuitQIREditorWidget qubits={2} circuitDepth={4} qirCode="define void @main() #0 { ret void }" />,
    );
    expect(screen.container.querySelector('[data-provenance="measured"]')).not.toBeNull();
    assertMetricsCarryProvenance(screen.container);
  });

  it('tags manifold geometry metrics with measured provenance', async () => {
    const screen = await render(
      <EpistemicManifoldSurfaceWidget currentCurvatureScalar={-0.4} fisherInfoNorm={2.5} entropyReduction={0.2} />,
    );
    expect(screen.container.querySelector('[data-provenance="measured"]')).not.toBeNull();
    assertMetricsCarryProvenance(screen.container);
  });

  it('gives every workbench object a controlled provenance value', () => {
    const objects = buildWorkbench(campaign);
    expect(objects.length).toBeGreaterThan(0);
    for (const object of objects) {
      expect(['measured', 'proposed', 'catalog', 'model']).toContain(object.provenance);
    }
  });
});
