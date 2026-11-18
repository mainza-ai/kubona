// Oversight domain boundary (approvals, tool approvals, agentic digest, mode).
// Components consume this facade; transport stays in lib/api.
export {
  decideApproval,
  decideToolApproval,
  fetchAgenticDigest,
  fetchApprovals,
  fetchSessionAgentic,
  fetchToolApprovals,
  setSessionAgentic,
} from '@/lib/api';
export type {
  AgenticDigest,
  AgenticDigestEntry,
  ApprovalPlan,
  PlanEdits,
  SessionAgenticState,
  ToolApproval,
} from '@/lib/api';
