import { describe, expect, it } from 'vitest';
import type { Job } from '@/types';
import { classifyJobTargetScope, matchesCampaignMoleculeTarget } from './target-scope';

function makeJob(overrides: Partial<Job>): Job {
  return {
    id: 'job-1',
    session_id: 'sess-1',
    status: 'JOB_STATUS_COMPLETED',
    job_type: 'vqe_run',
    params_json: {},
    result_json: {},
    created_at: '2026-11-11T10:00:00Z',
    completed_at: '2026-11-11T10:01:00Z',
    ...overrides,
  };
}

describe('matchesCampaignMoleculeTarget', () => {
  it('enforces whole-token matching without short substring collisions', () => {
    expect(matchesCampaignMoleculeTarget({ targetMolecule: 'Hydrogen Molecule (H2)', molecularFormula: 'H2' }, 'H2')).toBe(true);
    expect(matchesCampaignMoleculeTarget({ targetMolecule: 'Lithium Hydride (LiH)' }, 'Li')).toBe(false);
    expect(matchesCampaignMoleculeTarget({ targetMolecule: 'Hydrogen Molecule (H2)' }, 'LiH')).toBe(false);
    expect(matchesCampaignMoleculeTarget({ targetMolecule: 'Hydrogen Molecule (H2)' }, 'H')).toBe(false);
  });
});

describe('classifyJobTargetScope', () => {
  it('remains permissive when no scope is supplied', () => {
    const lih = makeJob({
      result_json: { molecule: 'LiH', energy: -7.88, fci_exact_energy: -7.88236 },
    });
    expect(classifyJobTargetScope(lih)).toEqual({ matches: true });
  });

  it('rejects explicit wrong-molecule jobs and FCI-incompatible jobs', () => {
    const h2Scope = { targetMolecule: 'Hydrogen Molecule (H2)', molecularFormula: 'H2', anchorFci: -1.13727 };
    expect(
      classifyJobTargetScope(
        makeJob({ job_type: 'vqe_run', result_json: { molecule: 'LiH', energy: -7.88, fci_exact_energy: -7.88236 } }),
        h2Scope
      )
    ).toEqual({ matches: false, reason: 'molecule mismatch' });

    expect(
      classifyJobTargetScope(
        makeJob({ job_type: 'vqe_pes', result_json: { points: [{ bond_distance: 1.5, energy: -7.881, fci_exact_energy: -7.88236 }] } }),
        h2Scope
      )
    ).toEqual({ matches: false, reason: 'fci anchor mismatch' });

    expect(
      classifyJobTargetScope(
        makeJob({ job_type: 'vqe_run', result_json: { molecule: 'H2', energy: -1.1368, fci_exact_energy: -1.13727 } }),
        h2Scope
      )
    ).toEqual({ matches: true });
  });

  it('excludes spin, DMRG, conjecture, and QAOA jobs from a molecular H2 campaign unless target matches or goal allows them', () => {
    const h2Scope = {
      targetMolecule: 'Hydrogen Molecule (H2)',
      molecularFormula: 'H2',
      researchGoal: 'Live QPU loop acceptance',
      anchorFci: -1.13727,
    };
    expect(classifyJobTargetScope(makeJob({ job_type: 'spin_chain', params_json: { model: 'tfim' } }), h2Scope)).toEqual({
      matches: false,
      reason: 'domain mismatch',
    });
    expect(classifyJobTargetScope(makeJob({ job_type: 'thermodynamic_limit', params_json: { model: 'tfim' } }), h2Scope)).toEqual({
      matches: false,
      reason: 'domain mismatch',
    });
    expect(classifyJobTargetScope(makeJob({ job_type: 'conjecture_run' }), h2Scope)).toEqual({
      matches: false,
      reason: 'domain mismatch',
    });
    expect(
      classifyJobTargetScope(
        makeJob({ job_type: 'circuit_run', result_json: { counts: { '01': 2 }, qaoa_meta: { optimal_cut: 2 } } }),
        h2Scope
      )
    ).toEqual({
      matches: false,
      reason: 'domain mismatch',
    });

    // Spin-chain campaign allows spin/DMRG jobs cleanly.
    const spinScope = {
      researchGoal: 'transverse-field Ising spin chain',
      targetSystem: 'TFIM spin chain',
    };
    expect(classifyJobTargetScope(makeJob({ job_type: 'spin_chain', params_json: { model: 'tfim' } }), spinScope)).toEqual({
      matches: true,
    });

    // Explicit target tag keeps pinned diagnostic jobs scoped to H2.
    expect(
      classifyJobTargetScope(makeJob({ job_type: 'thermodynamic_limit', params_json: { target: 'H2' } }), h2Scope)
    ).toEqual({ matches: true });
  });
});
