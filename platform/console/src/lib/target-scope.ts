import { asRecord } from '@/lib/guards';
import type { Job } from '@/types';

export interface CampaignTargetScope {
  targetMolecule?: string | null;
  molecularFormula?: string | null;
  targetSystem?: string | null;
  researchGoal?: string | null;
  anchorFci?: number | null;
}

export interface TargetScopeDecision {
  matches: boolean;
  reason?: string;
}

const NON_MOLECULAR_JOB_TYPES = new Set([
  'spin_chain',
  'spin_chain_hardware',
  'thermodynamic_limit',
  'lattice_dynamics',
  'qaoa_run',
  'qaoa_hardware',
  'conjecture_run',
]);

function num(v: unknown): number | null {
  return typeof v === 'number' && Number.isFinite(v) ? v : null;
}

function str(v: unknown): string {
  return typeof v === 'string' ? v.trim() : '';
}

function tokenizeAlphaNum(upper: string): string[] {
  return upper.split(/[^A-Z0-9]+/).filter(Boolean);
}

/**
 * Token-bounded molecule comparison mirroring backend matchesCampaignMolecule:
 * ignores tokens shorter than 2 chars so "H" never matches arbitrary strings.
 */
export function matchesCampaignMoleculeTarget(
  scope: Pick<CampaignTargetScope, 'targetMolecule' | 'molecularFormula' | 'targetSystem' | 'researchGoal'>,
  candidateMolecule: string
): boolean {
  const norm = candidateMolecule.trim().toUpperCase();
  if (norm.length < 2) return false;
  const formula = str(scope.molecularFormula).toUpperCase();
  if (formula && formula === norm) return true;
  const haystacks = [str(scope.targetMolecule), str(scope.targetSystem), str(scope.researchGoal)]
    .map((v) => v.toUpperCase())
    .filter(Boolean);
  for (const h of haystacks) {
    if (tokenizeAlphaNum(h).includes(norm)) return true;
  }
  return false;
}

function goalAllowsNonMolecularType(jobType: string, goal: string): boolean {
  if (!goal) return false;
  if (jobType === 'spin_chain' || jobType === 'spin_chain_hardware' || jobType === 'thermodynamic_limit' || jobType === 'lattice_dynamics') {
    return /\b(spin|ising|tfim|heisenberg|lattice|quench|dmrg|thermodynamic)\b/i.test(goal);
  }
  if (jobType === 'qaoa_run' || jobType === 'qaoa_hardware') {
    return /\b(qaoa|maxcut|combinatorial|graph)\b/i.test(goal);
  }
  if (jobType === 'conjecture_run') {
    return /\b(conjecture|sequence|ramanujan|theorem|lean|math)\b/i.test(goal);
  }
  return false;
}

function extractJobMolecule(params: Record<string, unknown>, result: Record<string, unknown>): string {
  return str(result.molecule) || str(params.molecule) || str(params.target) || str(result.target);
}

function extractJobFciAnchor(params: Record<string, unknown>, result: Record<string, unknown>): number | null {
  const direct =
    num(result.fci_exact_energy) ??
    num(result.classical_fci_energy) ??
    num(result.ground_energy_exact) ??
    num(params.fci_exact_energy);
  if (direct !== null && direct !== 0) return direct;
  if (Array.isArray(result.points) && result.points.length > 0) {
    const first = asRecord(result.points[0]);
    const ptFci = num(first.fci_exact_energy);
    if (ptFci !== null && ptFci !== 0) return ptFci;
  }
  return null;
}

/**
 * Classifies whether a historical or active session job belongs to the active
 * campaign's molecular/domain target rather than an adjacent experiment.
 */
export function classifyJobTargetScope(job: Job, scope?: CampaignTargetScope): TargetScopeDecision {
  if (!scope) return { matches: true };
  const hasMoleculeScope = Boolean(str(scope.targetMolecule) || str(scope.molecularFormula));
  const hasAnchorScope = typeof scope.anchorFci === 'number' && Number.isFinite(scope.anchorFci) && scope.anchorFci !== 0;
  const goal = str(scope.researchGoal) || str(scope.targetSystem);
  if (!hasMoleculeScope && !hasAnchorScope && !goal) {
    return { matches: true };
  }

  const params = asRecord(job.params_json);
  const result = asRecord(job.result_json);
  const jobMol = extractJobMolecule(params, result);

  if (jobMol && (hasMoleculeScope || goal)) {
    if (!matchesCampaignMoleculeTarget(scope, jobMol)) {
      return { matches: false, reason: 'molecule mismatch' };
    }
    // Explicit molecule/target attribute matches campaign target cleanly.
  }

  const jobFci = extractJobFciAnchor(params, result);
  if (hasAnchorScope && jobFci !== null) {
    const anchor = scope.anchorFci as number;
    if (Math.abs(jobFci - anchor) > Math.max(1.0, 0.5 * Math.abs(anchor))) {
      return { matches: false, reason: 'fci anchor mismatch' };
    }
  }

  const nonMolDomain = NON_MOLECULAR_JOB_TYPES.has(job.job_type) || (job.job_type === 'circuit_run' && Boolean(result.qaoa_meta));
  if (nonMolDomain && hasMoleculeScope) {
    if (jobMol && matchesCampaignMoleculeTarget(scope, jobMol)) {
      return { matches: true };
    }
    const effectiveType = job.job_type === 'circuit_run' && result.qaoa_meta ? 'qaoa_run' : job.job_type;
    if (!goalAllowsNonMolecularType(effectiveType, goal)) {
      return { matches: false, reason: 'domain mismatch' };
    }
  }

  return { matches: true };
}
