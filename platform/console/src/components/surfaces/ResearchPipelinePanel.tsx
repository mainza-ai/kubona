'use client';

import React, { useCallback, useEffect, useState } from 'react';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { EmptyState } from '@/components/ui/EmptyState';
import { Field, Select } from '@/components/ui/Forms';
import {
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
  type ExperimentPlanRecord,
  type HypothesisRecord,
  type LabKnowledgeEntry,
  type ResearchGraphPayload,
} from '@/domain/research';

type PipelineTab = 'hypotheses' | 'plans' | 'graph' | 'knowledge';

export function ResearchPipelinePanel({ sessionId }: { sessionId: string | null }) {
  const [tab, setTab] = useState<PipelineTab>('hypotheses');
  const [hypotheses, setHypotheses] = useState<HypothesisRecord[]>([]);
  const [plans, setPlans] = useState<ExperimentPlanRecord[]>([]);
  const [graph, setGraph] = useState<ResearchGraphPayload | null>(null);
  const [knowledge, setKnowledge] = useState<LabKnowledgeEntry[]>([]);
  const [goal, setGoal] = useState('');
  const [useLLM, setUseLLM] = useState(false);
  const [planHypId, setPlanHypId] = useState('');
  const [planStatement, setPlanStatement] = useState('');
  const [planMethod, setPlanMethod] = useState('vqe');
  const [planObservable, setPlanObservable] = useState('energy');
  const [planMetric, setPlanMetric] = useState('fci_exact_energy');
  const [planTolerance, setPlanTolerance] = useState('0.01');
  const [planStopRule, setPlanStopRule] = useState('stop after 1 measured run');
  const [planBudget, setPlanBudget] = useState('1');
  const [knowledgeQuery, setKnowledgeQuery] = useState('');
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const loadAll = useCallback(async () => {
    if (!sessionId) return;
    const [h, p, g, k] = await Promise.all([
      fetchHypotheses(sessionId),
      fetchPlans(sessionId),
      fetchResearchGraph(sessionId),
      fetchLabKnowledge(sessionId, knowledgeQuery.trim() || undefined),
    ]);
    if (h) setHypotheses(h.hypotheses ?? []);
    if (p) setPlans(p.plans ?? []);
    if (g) setGraph(g);
    if (k) setKnowledge(k.entries ?? []);
  }, [sessionId, knowledgeQuery]);

  useEffect(() => {
    if (!sessionId) return;
    let active = true;
    void Promise.all([
      fetchHypotheses(sessionId),
      fetchPlans(sessionId),
      fetchResearchGraph(sessionId),
      fetchLabKnowledge(sessionId),
    ]).then(([h, p, g, k]) => {
      if (!active) return;
      if (h) setHypotheses(h.hypotheses ?? []);
      if (p) setPlans(p.plans ?? []);
      if (g) setGraph(g);
      if (k) setKnowledge(k.entries ?? []);
    });
    return () => { active = false; };
  }, [sessionId]);

  if (!sessionId) {
    return <EmptyState title="No session">Select a session to manage hypotheses, plans, and lab knowledge.</EmptyState>;
  }

  const handleGenerate = async () => {
    setBusy('generate');
    setError(null);
    setNotice(null);
    const res = await generateHypotheses(sessionId, {
      goal: goal.trim() || undefined,
      count: 6,
      use_llm: useLLM,
    });
    if (!res.ok) setError(res.error);
    else {
      setHypotheses(res.data.hypotheses ?? []);
      setNotice(`Generated ${res.data.hypotheses?.length ?? 0} hypothesis candidate(s).`);
    }
    setBusy(null);
  };

  const handleTournament = async () => {
    setBusy('tournament');
    setError(null);
    setNotice(null);
    const res = await runHypothesisTournament(sessionId);
    if (!res.ok) setError(res.error);
    else {
      if (Array.isArray(res.data.hypotheses) && res.data.hypotheses.length > 0) {
        setHypotheses(res.data.hypotheses);
      } else {
        await loadAll();
      }
      setNotice('Elo tournament completed.');
    }
    setBusy(null);
  };

  const handleHypStatus = async (id: string, status: string) => {
    setBusy(`hyp-${id}`);
    setError(null);
    setNotice(null);
    const res = await setHypothesisStatus(sessionId, id, status);
    if (!res.ok) setError(res.error);
    else {
      setNotice(`Hypothesis marked ${status}.`);
      await loadAll();
    }
    setBusy(null);
  };

  const handleCollapse = async (h: HypothesisRecord) => {
    const completedPlan = plans.find((p) => p.hypothesis_id === h.id && p.status === 'completed');
    setBusy(`collapse-${h.id}`);
    setError(null);
    setNotice(null);
    const res = await collapseHypothesis(sessionId, h.id, completedPlan ? { plan_id: completedPlan.id } : {});
    if (!res.ok) setError(res.error);
    else {
      setNotice('Hypothesis collapsed with evidence.');
      await loadAll();
    }
    setBusy(null);
  };

  const handleCreatePlan = async () => {
    const tol = Number(planTolerance);
    const bud = Number(planBudget);
    if (!Number.isFinite(tol) || tol < 0) {
      setError('Tolerance must be >= 0.');
      return;
    }
    if (!Number.isFinite(bud) || bud < 0) {
      setError('Budget must be >= 0.');
      return;
    }
    setBusy('create-plan');
    setError(null);
    setNotice(null);
    const res = await createPlan(sessionId, {
      hypothesis_id: planHypId || undefined,
      statement: planStatement.trim() || undefined,
      method: planMethod,
      observable: planObservable.trim() || 'energy',
      acceptance_metric: planMetric.trim() || 'fci_exact_energy',
      tolerance: tol,
      stop_rule: planStopRule.trim() || 'stop after 1 measured run',
      budget_usd: bud,
      ...(planHypId ? {} : { spec: { kind: planMethod, molecule: 'H2', bond_distance: 0.7414, layers: 2, shots: 4096, max_iterations: 40 } }),
    });
    if (!res.ok) setError(res.error);
    else {
      setNotice(`Pre-registered plan ${res.data.plan?.id ?? ''} created.`);
      setPlanStatement('');
      await loadAll();
    }
    setBusy(null);
  };

  const handlePlanStatus = async (id: string, status: 'approved' | 'rejected') => {
    setBusy(`plan-${id}`);
    setError(null);
    setNotice(null);
    const res = await setPlanStatus(sessionId, id, status);
    if (!res.ok) setError(res.error);
    else {
      setNotice(`Plan ${status}.`);
      await loadAll();
    }
    setBusy(null);
  };

  const handleExecutePlan = async (id: string) => {
    setBusy(`exec-${id}`);
    setError(null);
    setNotice(null);
    const res = await executePlan(sessionId, id);
    if (!res.ok) setError(res.error);
    else {
      setNotice(`Plan execution finished (${res.data.status}).`);
      await loadAll();
    }
    setBusy(null);
  };

  const shortlisted = hypotheses.filter((h) => h.status === 'shortlisted' || h.status === 'collapsed');

  return (
    <section className="rounded-card border border-border bg-surface p-4 space-y-4" aria-label="Research pipeline">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 className="text-title font-semibold text-text">Research Pipeline</h3>
          <p className="text-label text-muted">
            Hypotheses, Elo ranking, pre-registered plans, research graph, and cumulative lab knowledge.
          </p>
        </div>
        <div className="flex flex-wrap gap-1.5">
          {(['hypotheses', 'plans', 'graph', 'knowledge'] as const).map((key) => (
            <Button
              key={key}
              size="compact"
              variant={tab === key ? 'primary' : 'ghost'}
              onClick={() => setTab(key)}
            >
              {key === 'hypotheses'
                ? `Hypotheses (${hypotheses.length})`
                : key === 'plans'
                  ? `Plans (${plans.length})`
                  : key === 'graph'
                    ? 'Research Graph'
                    : `Lab Knowledge (${knowledge.length})`}
            </Button>
          ))}
          <Button size="compact" variant="ghost" onClick={() => void loadAll()}>
            Refresh
          </Button>
        </div>
      </div>

      {error && (
        <p role="alert" className="text-label text-danger-ink border border-danger-edge bg-danger-wash rounded-control px-3 py-2">
          {error}
        </p>
      )}
      {notice && <p role="status" className="text-label text-muted">{notice}</p>}

      {tab === 'hypotheses' && (
        <div className="space-y-3">
          <div className="flex flex-wrap items-end gap-2">
            <Field
              label="Research goal (optional)"
              value={goal}
              onChange={(e) => setGoal(e.target.value)}
              placeholder="e.g. close H2 STO-3G VQE discrepancy below 1.6 mHa"
              className="flex-1 min-w-48"
            />
            <Select
              label="Proposer"
              value={useLLM ? 'llm' : 'diagnostic'}
              onChange={(e) => setUseLLM(e.target.value === 'llm')}
              options={[
                { value: 'diagnostic', label: 'Diagnostic floor' },
                { value: 'llm', label: 'Diagnostic + LLM' },
              ]}
            />
            <Button size="compact" variant="primary" pending={busy === 'generate'} onClick={() => void handleGenerate()}>
              Generate hypotheses
            </Button>
            <Button
              size="compact"
              variant="secondary"
              pending={busy === 'tournament'}
              disabled={hypotheses.length < 2}
              onClick={() => void handleTournament()}
            >
              Run Elo tournament
            </Button>
          </div>

          {hypotheses.length === 0 ? (
            <EmptyState title="No hypotheses yet">Generate falsifiable probes from the measured campaign state.</EmptyState>
          ) : (
            <ul className="space-y-2">
              {hypotheses.map((h) => (
                <li key={h.id} className="rounded-row border border-border bg-card p-3 space-y-1.5 text-label">
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div className="font-medium text-text">{h.statement}</div>
                    <div className="flex items-center gap-1.5 shrink-0">
                      <Badge tone={h.status === 'collapsed' ? 'success' : h.status === 'shortlisted' ? 'info' : 'neutral'}>
                        {h.status}
                      </Badge>
                      <Badge tone="neutral">score {Number(h.score ?? 0).toFixed(2)}</Badge>
                      {h.source && <Badge tone="neutral">{h.source}</Badge>}
                    </div>
                  </div>
                  {h.falsifier && <p className="text-caption text-muted">Falsifier: {h.falsifier}</p>}
                  <div className="flex flex-wrap items-center gap-1.5">
                    {h.status !== 'shortlisted' && h.status !== 'collapsed' && (
                      <Button size="compact" variant="secondary" pending={busy === `hyp-${h.id}`} onClick={() => void handleHypStatus(h.id, 'shortlisted')}>
                        Shortlist
                      </Button>
                    )}
                    {h.status !== 'rejected' && h.status !== 'collapsed' && (
                      <Button size="compact" variant="ghost" pending={busy === `hyp-${h.id}`} onClick={() => void handleHypStatus(h.id, 'rejected')}>
                        Reject
                      </Button>
                    )}
                    {h.status === 'shortlisted' && (
                      <Button size="compact" variant="primary" pending={busy === `collapse-${h.id}`} onClick={() => void handleCollapse(h)}>
                        Collapse with evidence
                      </Button>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      {tab === 'plans' && (
        <div className="space-y-3">
          <div className="rounded-row border border-border bg-card p-3 space-y-2">
            <div className="text-label font-semibold text-text">Pre-register experiment plan</div>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
              <Select
                label="Linked shortlisted hypothesis (optional)"
                value={planHypId}
                onChange={(e) => setPlanHypId(e.target.value)}
                options={[
                  { value: '', label: 'Standalone plan (explicit spec)' },
                  ...shortlisted.map((h) => ({ value: h.id, label: `${h.id}: ${h.statement.slice(0, 60)}` })),
                ]}
              />
              <Select
                label="Method"
                value={planMethod}
                onChange={(e) => setPlanMethod(e.target.value)}
                options={[
                  { value: 'vqe', label: 'vqe' },
                  { value: 'vqe_repeats', label: 'vqe_repeats' },
                  { value: 'pes', label: 'pes' },
                  { value: 'excited_state', label: 'excited_state' },
                  { value: 'spin_chain', label: 'spin_chain' },
                  { value: 'qaoa', label: 'qaoa' },
                ]}
              />
              <Field label="Statement" value={planStatement} onChange={(e) => setPlanStatement(e.target.value)} placeholder="Protocol statement" />
              <Field label="Observable" value={planObservable} onChange={(e) => setPlanObservable(e.target.value)} />
              <Field label="Acceptance metric" value={planMetric} onChange={(e) => setPlanMetric(e.target.value)} />
              <Field label="Tolerance" type="number" step="any" value={planTolerance} onChange={(e) => setPlanTolerance(e.target.value)} />
              <Field label="Stop rule" value={planStopRule} onChange={(e) => setPlanStopRule(e.target.value)} />
              <Field label="Budget (USD)" type="number" step="any" value={planBudget} onChange={(e) => setPlanBudget(e.target.value)} />
            </div>
            <Button size="compact" variant="primary" pending={busy === 'create-plan'} onClick={() => void handleCreatePlan()}>
              Register plan
            </Button>
          </div>

          {plans.length === 0 ? (
            <EmptyState title="No plans registered">Register a pre-registered experiment protocol above.</EmptyState>
          ) : (
            <ul className="space-y-2">
              {plans.map((p) => (
                <li key={p.id} className="rounded-row border border-border bg-card p-3 space-y-1.5 text-label">
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div>
                      <div className="font-medium text-text">{p.statement}</div>
                      <div className="text-caption font-code text-muted">
                        {p.id} · {p.method} · {p.observable} vs {p.acceptance_metric} (±{p.tolerance}) · ${p.budget_usd}
                      </div>
                    </div>
                    <div className="flex items-center gap-1.5">
                      <Badge tone={p.status === 'completed' ? 'success' : p.status === 'approved' ? 'info' : 'warning'}>
                        {p.status}
                      </Badge>
                      {p.risk && <Badge tone="neutral">{p.risk}</Badge>}
                    </div>
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {p.status === 'proposed' && (
                      <>
                        <Button size="compact" variant="secondary" pending={busy === `plan-${p.id}`} onClick={() => void handlePlanStatus(p.id, 'approved')}>
                          Approve
                        </Button>
                        <Button size="compact" variant="ghost" pending={busy === `plan-${p.id}`} onClick={() => void handlePlanStatus(p.id, 'rejected')}>
                          Reject
                        </Button>
                      </>
                    )}
                    {(p.status === 'approved' || p.status === 'proposed') && (
                      <Button size="compact" variant="primary" pending={busy === `exec-${p.id}`} onClick={() => void handleExecutePlan(p.id)}>
                        Execute plan
                      </Button>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      {tab === 'graph' && (
        <div className="space-y-2">
          {!graph ? (
            <EmptyState title="Research graph unavailable">No research graph projection loaded for this session.</EmptyState>
          ) : (
            <div className="space-y-2">
              <div className="flex flex-wrap gap-2 text-caption font-code text-muted">
                <span>Nodes: {graph.nodes?.length ?? 0}</span>
                <span>Edges: {graph.edges?.length ?? 0}</span>
              </div>
              <pre className="max-h-72 overflow-auto rounded-row border border-border bg-field p-3 font-code text-caption text-label-secondary">
                {JSON.stringify(graph, null, 2)}
              </pre>
            </div>
          )}
        </div>
      )}

      {tab === 'knowledge' && (
        <div className="space-y-3">
          <div className="flex gap-2 items-end">
            <Field
              label="Filter lab knowledge"
              value={knowledgeQuery}
              onChange={(e) => setKnowledgeQuery(e.target.value)}
              placeholder="Search negative results, claims, reports…"
              className="flex-1"
            />
            <Button size="compact" onClick={() => void loadAll()}>Search</Button>
          </div>
          {knowledge.length === 0 ? (
            <EmptyState title="No lab knowledge entries">Rejected plans, collapsed hypotheses, and signed manuscripts publish here automatically.</EmptyState>
          ) : (
            <ul className="space-y-2">
              {knowledge.map((entry) => (
                <li key={entry.id} className="rounded-row border border-border bg-card p-3 space-y-1 text-label">
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-medium text-text">{entry.title || entry.id}</span>
                    <Badge tone={entry.kind === 'negative_result' ? 'warning' : 'success'}>{entry.kind}</Badge>
                  </div>
                  <p className="text-caption text-muted">{entry.statement}</p>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </section>
  );
}
