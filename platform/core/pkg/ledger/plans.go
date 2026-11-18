package ledger

// Experiment plans (AG-3): a pre-registered protocol binding a hypothesis to
// one experiment family, an observable, an acceptance tolerance, and a stop
// rule. A plan is not evidence; executing it produces a job whose result is
// scored against the registered acceptance metric.

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const plansSchema = `
CREATE TABLE IF NOT EXISTS experiment_plans (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	tenant_id TEXT NOT NULL DEFAULT '',
	hypothesis_id TEXT NOT NULL DEFAULT '',
	statement TEXT NOT NULL,
	method TEXT NOT NULL,
	observable TEXT NOT NULL,
	acceptance_metric TEXT NOT NULL,
	tolerance REAL NOT NULL,
	stop_rule TEXT NOT NULL,
	budget_usd REAL NOT NULL DEFAULT 1.0,
	executor TEXT NOT NULL DEFAULT 'bridge',
	spec_json TEXT NOT NULL DEFAULT '{}',
	spec_basis_json TEXT NOT NULL DEFAULT '[]',
	risk TEXT NOT NULL DEFAULT '',
	risk_reasons_json TEXT NOT NULL DEFAULT '[]',
	risk_categories_json TEXT NOT NULL DEFAULT '[]',
	deferred_until TEXT NOT NULL DEFAULT '',
	defer_reason TEXT NOT NULL DEFAULT '',
	review_feedback TEXT NOT NULL DEFAULT '',
	reviewed_at TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'proposed',
	job_id TEXT NOT NULL DEFAULT '',
	result_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_experiment_plans_session
	ON experiment_plans(session_id, created_at DESC);`

const (
	PlanProposed  = "proposed"
	PlanApproved  = "approved"
	PlanRunning   = "running"
	PlanCompleted = "completed"
	PlanRejected  = "rejected"
	PlanFailed    = "failed"
)

type ExperimentPlan struct {
	ID               string   `json:"id"`
	SessionID        string   `json:"session_id"`
	HypothesisID     string   `json:"hypothesis_id,omitempty"`
	Statement        string   `json:"statement"`
	Method           string   `json:"method"`
	Observable       string   `json:"observable"`
	AcceptanceMetric string   `json:"acceptance_metric"`
	Tolerance        float64  `json:"tolerance"`
	StopRule         string   `json:"stop_rule"`
	BudgetUSD        float64  `json:"budget_usd"`
	Executor         string   `json:"executor"`
	SpecJSON         string   `json:"spec"`
	SpecBasis        []string `json:"spec_basis,omitempty"`
	Risk             string   `json:"risk,omitempty"`
	RiskReasons      []string `json:"risk_reasons,omitempty"`
	RiskCategories   []string `json:"risk_categories,omitempty"`
	DeferredUntil    string   `json:"deferred_until,omitempty"`
	DeferReason      string   `json:"defer_reason,omitempty"`
	ReviewFeedback   string   `json:"review_feedback,omitempty"`
	ReviewedAt       string   `json:"reviewed_at,omitempty"`
	Status           string   `json:"status"`
	JobID            string   `json:"job_id,omitempty"`
	ResultJSON       string   `json:"result,omitempty"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

var ErrPlanNotFound = errors.New("experiment plan not found")

// ErrPlanNotPending refuses an edit to a plan that is no longer awaiting a
// decision; the reviewer's copy is stale.
var ErrPlanNotPending = errors.New("experiment plan is no longer proposed")

func validPlanStatus(v string) bool {
	switch v {
	case PlanProposed, PlanApproved, PlanRunning, PlanCompleted, PlanRejected, PlanFailed:
		return true
	}
	return false
}

func (l *Ledger) CreatePlan(sessionID, tenantID string, p *ExperimentPlan) error {
	if p == nil || p.ID == "" || p.Statement == "" || p.Method == "" {
		return fmt.Errorf("plan needs id, statement, and method")
	}
	if p.AcceptanceMetric == "" || p.Observable == "" || p.StopRule == "" {
		return fmt.Errorf("plan needs observable, acceptance metric, and stop rule")
	}
	if p.Status == "" {
		p.Status = PlanProposed
	}
	if !validPlanStatus(p.Status) {
		return fmt.Errorf("plan status %q is not proposed|approved|running|completed|rejected|failed", p.Status)
	}
	if p.BudgetUSD <= 0 {
		p.BudgetUSD = 1.0
	}
	if p.Executor == "" {
		p.Executor = "bridge"
	}
	if strings.TrimSpace(p.SpecJSON) == "" {
		p.SpecJSON = "{}"
	}
	if strings.TrimSpace(p.ResultJSON) == "" {
		p.ResultJSON = "{}"
	}
	basis, err := jsonStrings(p.SpecBasis)
	if err != nil {
		return err
	}
	riskReasons, err := jsonStrings(p.RiskReasons)
	if err != nil {
		return err
	}
	riskCategories, err := jsonStrings(p.RiskCategories)
	if err != nil {
		return err
	}
	now := nowUTC()
	_, err = l.DB().Exec(`INSERT INTO experiment_plans
		(id, session_id, tenant_id, hypothesis_id, statement, method, observable,
		 acceptance_metric, tolerance, stop_rule, budget_usd, executor, spec_json,
		 spec_basis_json, risk, risk_reasons_json, risk_categories_json,
		 deferred_until, defer_reason, status, job_id, result_json, created_at,
		 updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, sessionID, tenantID, p.HypothesisID, p.Statement, p.Method, p.Observable,
		p.AcceptanceMetric, p.Tolerance, p.StopRule, p.BudgetUSD, p.Executor, p.SpecJSON,
		basis, p.Risk, riskReasons, riskCategories, p.DeferredUntil, p.DeferReason,
		p.Status, p.JobID, p.ResultJSON, now, now)
	if err != nil {
		return fmt.Errorf("insert experiment plan: %w", err)
	}
	p.SessionID, p.CreatedAt, p.UpdatedAt = sessionID, now, now
	return nil
}

func (l *Ledger) ListPlans(sessionID string, limit int) ([]ExperimentPlan, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := l.DB().Query(`SELECT id, session_id, hypothesis_id, statement, method, observable,
		acceptance_metric, tolerance, stop_rule, budget_usd, executor, spec_json,
		spec_basis_json, risk, risk_reasons_json, risk_categories_json,
		deferred_until, defer_reason, review_feedback, reviewed_at, status,
		job_id, result_json, created_at, updated_at
		FROM experiment_plans WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExperimentPlan{}
	for rows.Next() {
		var p ExperimentPlan
		var basis, riskReasons, riskCategories string
		if err := rows.Scan(&p.ID, &p.SessionID, &p.HypothesisID, &p.Statement, &p.Method,
			&p.Observable, &p.AcceptanceMetric, &p.Tolerance, &p.StopRule, &p.BudgetUSD,
			&p.Executor, &p.SpecJSON, &basis, &p.Risk, &riskReasons, &riskCategories,
			&p.DeferredUntil, &p.DeferReason, &p.ReviewFeedback, &p.ReviewedAt,
			&p.Status, &p.JobID, &p.ResultJSON, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		_ = jsonUnmarshalStrings(basis, &p.SpecBasis)
		_ = jsonUnmarshalStrings(riskReasons, &p.RiskReasons)
		_ = jsonUnmarshalStrings(riskCategories, &p.RiskCategories)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (l *Ledger) GetPlan(sessionID, id string) (*ExperimentPlan, error) {
	var p ExperimentPlan
	var basis, riskReasons, riskCategories string
	err := l.DB().QueryRow(`SELECT id, session_id, hypothesis_id, statement, method, observable,
		acceptance_metric, tolerance, stop_rule, budget_usd, executor, spec_json,
		spec_basis_json, risk, risk_reasons_json, risk_categories_json,
		deferred_until, defer_reason, review_feedback, reviewed_at, status, job_id,
		result_json, created_at, updated_at
		FROM experiment_plans WHERE session_id = ? AND id = ?`, sessionID, id).Scan(
		&p.ID, &p.SessionID, &p.HypothesisID, &p.Statement, &p.Method, &p.Observable,
		&p.AcceptanceMetric, &p.Tolerance, &p.StopRule, &p.BudgetUSD, &p.Executor,
		&p.SpecJSON, &basis, &p.Risk, &riskReasons, &riskCategories, &p.DeferredUntil,
		&p.DeferReason, &p.ReviewFeedback, &p.ReviewedAt, &p.Status, &p.JobID,
		&p.ResultJSON, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPlanNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = jsonUnmarshalStrings(basis, &p.SpecBasis)
	_ = jsonUnmarshalStrings(riskReasons, &p.RiskReasons)
	_ = jsonUnmarshalStrings(riskCategories, &p.RiskCategories)
	return &p, nil
}

func (l *Ledger) SetPlanStatus(sessionID, id, status string) error {
	if !validPlanStatus(status) {
		return fmt.Errorf("plan status %q is not proposed|approved|running|completed|rejected|failed", status)
	}
	res, err := l.DB().Exec(`UPDATE experiment_plans SET status = ?,
		deferred_until = '', defer_reason = '', updated_at = ?
		WHERE session_id = ? AND id = ?`, status, nowUTC(), sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPlanNotFound
	}
	return nil
}

// SetPlanDeferral parks a proposed plan until a future time without changing
// its status; the approval inbox excludes it from the pending queue until the
// time passes. The timestamp is required and validated by the caller.
func (l *Ledger) SetPlanDeferral(sessionID, id, until, reason string) error {
	res, err := l.DB().Exec(`UPDATE experiment_plans SET deferred_until = ?, defer_reason = ?,
		updated_at = ? WHERE session_id = ? AND id = ?`,
		until, reason, nowUTC(), sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPlanNotFound
	}
	return nil
}

// UpdatePlanReview persists reviewer-editable plan fields plus the decision
// metadata. Status is deliberately separate: a decision and an edit land in one
// request but remain individually auditable. The caller has already validated
// the fields; this only writes them.
func (l *Ledger) UpdatePlanReview(sessionID string, p *ExperimentPlan) error {
	if p == nil {
		return fmt.Errorf("plan review needs a plan")
	}
	if strings.TrimSpace(p.Statement) == "" || strings.TrimSpace(p.Method) == "" ||
		strings.TrimSpace(p.Observable) == "" || strings.TrimSpace(p.AcceptanceMetric) == "" ||
		strings.TrimSpace(p.StopRule) == "" {
		return fmt.Errorf("reviewed plan needs statement, method, observable, acceptance metric, and stop rule")
	}
	if strings.TrimSpace(p.SpecJSON) == "" {
		p.SpecJSON = "{}"
	}
	riskReasons, err := jsonStrings(p.RiskReasons)
	if err != nil {
		return err
	}
	riskCategories, err := jsonStrings(p.RiskCategories)
	if err != nil {
		return err
	}
	res, err := l.DB().Exec(`UPDATE experiment_plans SET statement = ?, method = ?,
		observable = ?, acceptance_metric = ?, tolerance = ?, stop_rule = ?,
		budget_usd = ?, spec_json = ?, risk = ?, risk_reasons_json = ?,
		risk_categories_json = ?, review_feedback = ?, reviewed_at = ?, updated_at = ?
		WHERE session_id = ? AND id = ? AND status = 'proposed'`,
		p.Statement, p.Method, p.Observable, p.AcceptanceMetric, p.Tolerance,
		p.StopRule, p.BudgetUSD, p.SpecJSON, p.Risk, riskReasons, riskCategories,
		p.ReviewFeedback, p.ReviewedAt, nowUTC(), sessionID, p.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPlanNotPending
	}
	return nil
}

func (l *Ledger) SetPlanResult(sessionID, id, status, jobID, resultJSON string) error {
	if !validPlanStatus(status) {
		return fmt.Errorf("plan status %q is invalid", status)
	}
	if strings.TrimSpace(resultJSON) == "" {
		resultJSON = "{}"
	}
	res, err := l.DB().Exec(`UPDATE experiment_plans SET status = ?, job_id = ?, result_json = ?,
		updated_at = ? WHERE session_id = ? AND id = ?`,
		status, jobID, resultJSON, nowUTC(), sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPlanNotFound
	}
	return nil
}

func (l *Ledger) GetHypothesis(sessionID, id string) (*Hypothesis, error) {
	var h Hypothesis
	var lit, patch string
	var distance sql.NullFloat64
	err := l.DB().QueryRow(`SELECT id, session_id, statement, rationale, falsifier,
		experiment_kind, experiment_hint, literature_json, spec_patch_json, novelty_status,
		novelty_method, novelty_distance, score, status, source, claim_id, created_at, updated_at
		FROM hypotheses WHERE session_id = ? AND id = ?`, sessionID, id).Scan(
		&h.ID, &h.SessionID, &h.Statement, &h.Rationale, &h.Falsifier, &h.ExperimentKind,
		&h.ExperimentHint, &lit, &patch, &h.NoveltyStatus, &h.NoveltyMethod, &distance,
		&h.Score, &h.Status, &h.Source, &h.ClaimID, &h.CreatedAt, &h.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrHypothesisNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = jsonUnmarshalStrings(lit, &h.Literature)
	_ = jsonUnmarshalObject(patch, &h.SpecPatch)
	if distance.Valid {
		v := distance.Float64
		h.NoveltyDistance = &v
	}
	return &h, nil
}
