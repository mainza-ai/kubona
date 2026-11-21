// Research pipeline domain boundary (AG-2 Hypotheses, AG-3 Plans, AG-4 Graph, AG-9 Lab Knowledge).
export {
  collapseHypothesis,
  createPlan,
  executePlan,
  fetchHypotheses,
  fetchLabKnowledge,
  fetchPlans,
  fetchResearchGraph,
  generateHypotheses,
  runHypothesisTournament,
  setHypothesisStatus,
  setPlanStatus,
} from '@/lib/api';
export type {
  ExperimentPlanRecord,
  HypothesisRecord,
  LabKnowledgeEntry,
  ResearchGraphPayload,
} from '@/lib/api';
