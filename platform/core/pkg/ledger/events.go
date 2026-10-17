package ledger

// S6 — ledger event-type taxonomy (closed set).
//
// Threat (G10): untyped event strings drift — a typo'd event_type silently
// forks the audit trail, and new unreviewed types enter records without
// anyone deciding their shape. Every event_type emitted by Go code must be
// listed here; the conformance test (TestEventTypesRegistered, api
// package) exercises broad flows and asserts DISTINCT event_type ⊆ this
// set. Adding a type = adding a row here + its payload contract.
//
// Python-rail events (workflow_step, workflow_dry_run) live in the
// separate Python ledger file and are out of scope for this registry.

var KnownEventTypes = map[string]bool{
	"a2a_harness_eval":                  true,
	"a2a_qir_compile":                   true,
	"action_gate_downgrade_sim":         true,
	"annotation":                        true,
	"autoresearch_loop_drain":           true,
	"autoresearch_loop_reconciled":      true,
	"autoresearch_loop_slept":           true,
	"autoresearch_loop_started":         true,
	"autoresearch_loop_woken":           true,
	"autoresearch_stagnation_triggered": true,
	"campaign_replanned":                true,
	"campaign_synthesized":              true,
	"chat_cleared":                      true,
	"chat_feedback":                     true,
	"chat_message_appended":             true,
	"chat_rewritten":                    true,
	"custom_agent_deleted":              true,
	"custom_agent_registered":           true,
	"discovery_pipeline_evaluated":      true,
	"disk_blocked":                      true,
	"disk_resumed":                      true,
	"disk_warning":                      true,
	"fidelity_assessment":               true,
	"grant_issued":                      true,
	"grant_revoked":                     true,
	"hf_download_started":               true,
	"idempotency_complete_orphaned":     true,
	"job_queued":                     true,
	"job_cancelled":                  true,
	"job_settled":                    true,
	"job_started":                       true,
	"lease_issued":                      true,
	"lease_revoked":                     true,
	"llm_spend_estimated":               true,
	"loop_completed":                    true,
	"loop_diverged":                     true,
	"loop_killed":                       true,
	"loop_paused":                       true,
	"loop_registered":                   true,
	"loop_rescoped":                     true,
	"loop_resumed":                      true,
	"loop_rollback_executed":            true,
	"loop_snapshot_saved":               true,
	"loop_stalled_timeout":              true,
	"mandate_verified_client":           true,
	"mandate_verified_pqc_mlkem768":     true,
	"metasearch_applied":                true,
	"molecular_admet_profiled":          true,
	"peer_review":                       true,
	"proposal_generated":                true,
	"session_created":                   true,
	"session_deleted":                   true,
	"session_imported":                  true,
	"session_spend_released":            true,
	"session_spend_reserved":            true,
	"supervisor_abort":                  true,
	"task_watchdog_woken":               true,
	"tenant_admin_bootstrapped":         true,
	"tenant_admin_rotated":              true,
	"tenant_autonomy_granted":           true,
	"tenant_autonomy_revoked":           true,
	"tenant_ceiling_changed":            true,
	"tenant_created":                    true,
	"tenant_jobs_aborted":               true,
	"tenant_spend_released":             true,
	"tenant_spend_reserved":             true,
	"tenant_status_changed":             true,
	"tool_called":                       true,
	"tool_loop_breaker":                 true,
	"tool_loop_capped":                  true,
	"tribunal_commit":                   true,
	"tribunal_turn":                     true,
	"x402_payment_settled":              true,
}

// IsKnownEventType reports whether eventType is in the closed taxonomy.
// Append paths refuse unknown types (fail-closed taxonomy — a typo'd type
// silently forks the audit trail).
func IsKnownEventType(eventType string) bool {
	return KnownEventTypes[eventType]
}
