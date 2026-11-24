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
  fetchJobs,
  fetchLabKnowledge,
  fetchPlans,
  fetchResearchGraph,
  generateHypotheses,
  runHypothesisTournament,
  setHypothesisStatus,
  setPlanStatus,
  type ExperimentPlanRecord,
  type HypothesisRecord,
  type Job,
  type LabKnowledgeEntry,
  type ResearchGraphPayload,
} from '@/domain/research';

type PipelineTab = 'hypotheses' | 'plans' | 'graph' | 'knowledge' | 'qec';
const QEC_JOB_TYPES = new Set(['qec_code_eval', 'qec_distance', 'qec_memory_sim']);

function parseJobResult(job: Job): Record<string, unknown> {
  const raw = job.result_json;
  if (raw && typeof raw === 'object') return raw as Record<string, unknown>;
  if (typeof raw === 'string' && raw.trim()) {
    try {
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === 'object') return parsed as Record<string, unknown>;
    } catch {
      return {};
    }
  }
  return {};
}

export function ResearchPipelinePanel({ sessionId }: { sessionId: string | null }) {
  const [tab, setTab] = useState<PipelineTab>('hypotheses');
  const [hypotheses, setHypotheses] = useState<HypothesisRecord[]>([]);
  const [plans, setPlans] = useState<ExperimentPlanRecord[]>([]);
  const [graph, setGraph] = useState<ResearchGraphPayload | null>(null);
  const [knowledge, setKnowledge] = useState<LabKnowledgeEntry[]>([]);
  const [qecJobs, setQecJobs] = useState<Job[]>([]);
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
    const [h, p, g, k, jobs] = await Promise.all([
      fetchHypotheses(sessionId),
      fetchPlans(sessionId),
      fetchResearchGraph(sessionId),
      fetchLabKnowledge(sessionId, knowledgeQuery.trim() || undefined),
      fetchJobs(sessionId),
    ]);
    if (h) setHypotheses(h.hypotheses ?? []);
    if (p) setPlans(p.plans ?? []);
    if (g) setGraph(g);
    if (k) setKnowledge(k.entries ?? []);
    setQecJobs((jobs ?? []).filter((j) => QEC_JOB_TYPES.has(j.job_type)));
  }, [sessionId, knowledgeQuery]);

  useEffect(() => {
    if (!sessionId) return;
    let active = true;
    void Promise.all([
      fetchHypotheses(sessionId),
      fetchPlans(sessionId),
      fetchResearchGraph(sessionId),
      fetchLabKnowledge(sessionId),
      fetchJobs(sessionId),
    ]).then(([h, p, g, k, jobs]) => {
      if (!active) return;
      if (h) setHypotheses(h.hypotheses ?? []);
      if (p) setPlans(p.plans ?? []);
      if (g) setGraph(g);
      if (k) setKnowledge(k.entries ?? []);
      setQecJobs((jobs ?? []).filter((j) => QEC_JOB_TYPES.has(j.job_type)));
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
          {(['hypotheses', 'plans', 'graph', 'knowledge', 'qec'] as const).map((key) => (
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
                    : key === 'knowledge'
                      ? `Lab Knowledge (${knowledge.length})`
                      : `QEC Claims (${qecJobs.length})`}
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
                      {(h.novelty_status || typeof h.novelty_distance === 'number') && (
                        <Badge tone="neutral">
                          lexical novelty{h.novelty_status ? `: ${h.novelty_status}` : ''}{typeof h.novelty_distance === 'number' ? ` (${h.novelty_distance.toFixed(2)})` : ''}
                        </Badge>
                      )}
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
      {tab === 'qec' && (
        <div className="space-y-3" data-testid="qec-claims-panel">
          {qecJobs.length === 0 ? (
            <EmptyState title="No QEC discovery jobs yet">
              Run a QEC code evaluation, distance proof, or memory simulation in the Experiment Lab to populate the 7-gate discovery ledger.
            </EmptyState>
          ) : (
            <ul className="space-y-2">
              {qecJobs.map((job) => {
                const r = parseJobResult(job);
                const isMemory = job.job_type === 'qec_memory_sim';
                const hasCodeParams = r.n != null && r.k != null;
                const dProved = r.d_kind === 'proved';
                const hasLer = typeof r.ler === 'number';
                const gates: Array<{ label: string; pass: boolean | null; detail: string }> = isMemory
                  ? [
                      { label: 'G4 Circuit LER', pass: hasLer && typeof r.ci_low === 'number' && typeof r.ci_high === 'number', detail: hasLer ? `LER ${(r.ler as number).toFixed(4)}` : 'unmeasured' },
                      { label: 'G5 Held-out IFC', pass: r.held_out_verified === true ? true : null, detail: r.held_out_verified === true ? 'verifier sealed' : 'pending verifier' },
                    ]
                  : [
                      { label: 'G0 CSS valid', pass: r.valid === true || hasCodeParams, detail: hasCodeParams ? `[[${String(r.n)},${String(r.k)},${String(r.d ?? '?')}]]` : 'unverified' },
                      { label: 'G1 Indecomposable', pass: typeof r.decomposable === 'boolean' ? !r.decomposable : null, detail: r.decomposable === false ? 'indecomposable' : r.decomposable === true ? 'decomposable' : 'unchecked' },
                      { label: 'G2 Distance proved', pass: dProved, detail: dProved ? 'PROVED' : 'UPPER BOUND' },
                      { label: 'G3 Catalog novelty', pass: typeof r.catalog_known === 'boolean' ? !r.catalog_known : null, detail: r.catalog_known ? `known (${String(r.catalog_match ?? 'catalog')})` : r.catalog_known === false ? 'novel vs catalog' : 'unchecked' },
                      { label: 'G6 Canonical hash', pass: Boolean(r.canonical_hash), detail: r.canonical_hash ? String(r.canonical_hash).slice(0, 10) : 'missing' },
                      { label: 'G7 Formal / Lean', pass: r.lean_attestation_hash ? true : null, detail: r.lean_attestation_hash ? 'attested' : 'unsealed' },
                    ];
                return (
                  <li key={job.id} className="rounded-row border border-border bg-card p-3 space-y-2 text-label">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="font-code text-text flex flex-wrap items-center gap-1.5">
                        <span className="font-semibold">{job.job_type}</span>
                        <span className="text-muted">· {job.id.slice(0, 8)}</span>
                        {hasCodeParams && (
                          <span>· [[{String(r.n)}, {String(r.k)}, {String(r.d ?? '?')}]]</span>
                        )}
                        {!isMemory && hasCodeParams && (
                          <Badge tone={dProved ? 'success' : 'warning'}>
                            {dProved ? 'PROVED' : 'UPPER BOUND'}
                          </Badge>
                        )}
                        {typeof r.fom === 'number' && <span>· FOM {r.fom.toFixed(2)}</span>}
                        {isMemory && hasLer && (
                          <span>
                            · LER {(r.ler as number).toFixed(4)}
                            {typeof r.ci_low === 'number' && typeof r.ci_high === 'number'
                              ? ` [${r.ci_low.toFixed(4)}, ${r.ci_high.toFixed(4)}]`
                              : ''}
                            {r.d_circ != null ? ` · d_circ ${String(r.d_circ)}` : ''}
                            {typeof r.decoder_latency_us === 'number' ? ` · ${r.decoder_latency_us.toFixed(1)} µs/shot` : ''}
                          </span>
                        )}
                      </div>
                      <Badge tone={String(job.status).includes('COMPLETED') ? 'success' : String(job.status).includes('FAILED') ? 'danger' : 'info'}>
                        {String(job.status).replace(/^JOB_STATUS_/, '')}
                      </Badge>
                    </div>
                    <div className="flex flex-wrap gap-1.5">
                      {gates.map((g) => (
                        <Badge key={g.label} tone={g.pass === true ? 'success' : g.pass === false ? 'warning' : 'neutral'}>
                          {g.label}: {g.detail}
                        </Badge>
                      ))}
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      )}
    </section>
  );
}
