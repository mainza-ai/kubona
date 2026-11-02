import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import type { ReactTestInstance } from 'react-test-renderer';
import {
  MolecularActiveSpaceWidget,
  QuantumCircuitQIREditorWidget,
  EpistemicManifoldSurfaceWidget,
} from '@/components/widgets';
import { buildWorkbench } from '@/components/workbench/workbench-model';
import type { CampaignState } from '@/types';

function hasProvenanceAncestor(node: ReactTestInstance): boolean {
  let current: ReactTestInstance | null = node;
  while (current) {
    if (typeof current.props?.['data-provenance'] === 'string') return true;
    current = current.parent;
  }
  return false;
}

function assertMetricsCarryProvenance(tree: TestRenderer.ReactTestRenderer) {
  const metrics = tree.root.findAll((node) => node.props?.['data-metric'] !== undefined);
  expect(metrics.length).toBeGreaterThan(0);
  for (const metric of metrics) {
    expect(hasProvenanceAncestor(metric), `metric without provenance: ${JSON.stringify(metric.props.children)}`).toBe(true);
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

describe('provenance coverage', () => {
  it('tags molecular proposal metrics with proposed provenance', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <MolecularActiveSpaceWidget moleculeName="H2" numElectrons={2} numOrbitals={2} homoLumoGapHa={0.05} />
      );
    });
    expect(tree!.root.findByProps({ 'data-provenance': 'proposed' })).toBeTruthy();
    assertMetricsCarryProvenance(tree!);
  });

  it('tags QIR receipt metrics with measured provenance', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <QuantumCircuitQIREditorWidget qubits={2} circuitDepth={4} qirCode="define void @main() #0 { ret void }" />
      );
    });
    expect(tree!.root.findByProps({ 'data-provenance': 'measured' })).toBeTruthy();
    assertMetricsCarryProvenance(tree!);
  });

  it('tags manifold geometry metrics with measured provenance', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <EpistemicManifoldSurfaceWidget currentCurvatureScalar={-0.4} fisherInfoNorm={2.5} entropyReduction={0.2} />
      );
    });
    expect(tree!.root.findByProps({ 'data-provenance': 'measured' })).toBeTruthy();
    assertMetricsCarryProvenance(tree!);
  });

  it('gives every workbench object a controlled provenance value', () => {
    const objects = buildWorkbench(campaign);
    expect(objects.length).toBeGreaterThan(0);
    for (const object of objects) {
      expect(['measured', 'proposed', 'catalog', 'model']).toContain(object.provenance);
    }
  });
});
