// Oversight domain boundary (approvals, tool approvals, agentic digest, mode).
// Components consume this facade; transport stays in lib/api.
export {
  decideApproval,
  decideToolApproval,
  fetchAgenticDigest,
  fetchApprovals,
  fetchAutonomyHistory,
  fetchAutonomyPolicy,
  fetchScienceTelemetry,
  fetchSessionAgentic,
  fetchSessionReceipts,
  fetchTenantAgenticDefault,
  fetchToolApprovals,
  haltAutonomy,
  rollbackAutonomyPolicy,
  setSessionAgentic,
  setTenantAgenticDefault,
  undoSessionReceipt,
  updateAutonomyPolicy,
} from '@/lib/api';
export type {
  ActionReceipt,
  AgenticDigest,
  AgenticDigestEntry,
  ApprovalPlan,
  AutonomyHistoryEntry,
  AutonomyPolicyRecord,
  PlanEdits,
  SessionAgenticState,
  ToolApproval,
} from '@/lib/api';
