// Response contracts for the live console boundary.
//
// These are intentionally small structural schemas, not a full OpenAPI
// generator: they pin the fields the console reads and reject a payload that
// drifts underneath a typed wrapper. Extra backend fields stay legal. Drift is
// reported once per contract name and never replaces the payload — callers
// keep their existing empty/refused fallbacks.

export type SchemaType = 'object' | 'array' | 'string' | 'number' | 'boolean' | 'union' | 'any';

export interface ResponseSchema {
  type: SchemaType;
  required?: string[];
  properties?: Record<string, ResponseSchema>;
  items?: ResponseSchema;
  nullable?: boolean;
  /** Exactly one member's validation must pass. Used for legacy wire unions. */
  anyOf?: ResponseSchema[];
}

const any: ResponseSchema = { type: 'any' };
const str: ResponseSchema = { type: 'string' };
const num: ResponseSchema = { type: 'number' };
const bool: ResponseSchema = { type: 'boolean' };
const obj = (
  properties: Record<string, ResponseSchema> = {},
  required: string[] = Object.keys(properties),
): ResponseSchema => ({ type: 'object', properties, required });
const arr = (items: ResponseSchema): ResponseSchema => ({ type: 'array', items });
const union = (...anyOf: ResponseSchema[]): ResponseSchema => ({ type: 'union', anyOf });
// The Go API serializes the proto JobStatus enum as a number; older/test
// payloads may still carry the DB string (JOB_STATUS_*). Both are the same
// closed enum, not drift.
const jobStatus = union(num, str);

export const RESPONSE_SCHEMAS = {
  'sessions.list': obj({
    sessions: arr(obj({ id: str, title: str, status: str, created_at: str, updated_at: str })),
  }),
  session: obj({
    id: str,
    title: str,
    status: str,
    created_at: str,
    updated_at: str,
  }),
  'loops.list': obj({
    loops: arr(obj({ id: str, session_id: str, status: str, step_count: num, max_steps: num })),
  }),
  'events.list': obj({
    events: arr(obj({ id: num, timestamp: str, session_id: str, event_type: str, payload: obj() })),
  }),
  fleet: obj({
    backends: arr(any),
  }),
  capabilities: obj({
    catalog_digest: str,
    harnesses: arr(str),
    budget: obj(),
    flags: obj(),
  }),
  'canvas.layout': obj(),
  'workbench.layout': obj({ session_id: str, revision: num, document: obj() }),
  'workbench.save': obj({ status: str, session_id: str, revision: num }),
  'obligations.open': obj({
    obligations: arr(
      obj({
        id: str,
        subject_kind: str,
        subject_id: str,
        owner: str,
        disposition: str,
        close_ready: bool,
      }),
    ),
  }),
  'llm.settings': obj(),
  'quantum.settings': obj(),
  'quantum.job': obj(),
  'quantum.submit': obj({
    ok: bool,
  }),
  'vault.lease': obj({
    lease: obj({
      lease_id: str,
      scope: str,
      ttl_seconds: num,
      lease_signature: str,
      issued_at: str,
      expires_at: str,
    }),
  }),
  'ledger.verify': obj({
    ok: bool,
  }),
  'lens.request': obj({
    ok: bool,
  }),
  'lens.resolve': obj({
    ok: bool,
  }),
  'lens.history': obj({
    ok: bool,
    operations: arr(any),
  }),
  'lens.status': obj(),
  'vault.latest': obj({
    ok: bool,
  }),
  'vault.calibration': obj({
    ok: bool,
  }),
  'vault.history': obj({
    ok: bool,
    predictions: arr(any),
  }),
  'chat.send': obj({
    user: any,
    assistant: any,
  }),
  'chat.stream.done': obj({
    user: any,
    assistant: any,
  }),
  'tribunal.run': obj({
    tribunal: obj({ turns: arr(any) }),
    assistant_message: any,
  }),
  'chat.launch': obj({
    job: any,
    loop: any,
  }),
  'autoresearch.harnesses': obj({
    harnesses: arr(any),
    docker_runner: any,
  }),
  'autoresearch.loop': obj({
    id: str,
    session_id: str,
    status: str,
  }),
  'autoresearch.loops': obj({
    loops: arr(any),
  }),
  'autoresearch.step': obj({
    step: any,
    state: any,
  }),
  'metasearch.proposals': obj({
    proposals: arr(any),
  }),
  'codesign.evaluate': obj({
    ok: bool,
    spec_kind: str,
    result: obj(),
  }),
  'codesign.batch': obj({
    ok: bool,
    spec_kind: str,
    frontier: obj(),
    details: arr(any),
  }),
  'codesign.verified': obj({
    total: obj({ toffoli: num }),
  }),
  'codesign.packs': obj({
    ok: bool,
    packs: arr(any),
  }),
  'codesign.families': obj({
    ok: bool,
  }),
  'codesign.rerun': obj({
    ok: bool,
  }),
  'llm.models': obj({
    models: arr(any),
  }),
  'hf.search': obj({
    models: arr(any),
  }),
  'hf.downloads': obj({
    downloads: arr(any),
  }),
  'hf.details': obj({
    model: any,
  }),
  'hf.download': obj({
    task: any,
  }),
  'auth.login': obj({ ok: bool, tenant_id: str, pubkey_fp: str }),
  'auth.sessions': obj({ sessions: arr(any) }),
  'agent.one': obj({ agent: any }),
  lab: obj({ id: str, display_name: str, status: str }),
  'session.create': obj({ id: str, title: str, status: str, created_at: str, updated_at: str }),
  abort: obj({ aborted_count: num, abort_receipt: str }),
  discovery: obj(),
  campaign: obj({ session_id: str, quantum_factors: obj(), classical_anchor: obj(), trajectory: arr(any), updated_at: str }),
  epistemics: obj(),
  fidelity: obj({ admitted: bool, predicted_fidelity: num, fidelity_floor: num, calibration_fresh: bool, recommended_action: str }),
  admet: obj({ molecular_weight: num, clogp: num, tpsa: num }),
  'ledger.append': obj({ ok: bool, record_hash: any }),
  'ledger.export': obj({ events: arr(any) }),
  'memory.retain': obj({ id: str, bank: str, importance: num }),
  'mandate.sign': obj({ mandate: any, public_key: str, verified: bool }),
  'mandate.verify': obj({ valid: bool }),
  'tribunal.resume': obj({ tribunal: any, assistant_message: any }),
  'mesh.route': obj({ decision: any }),
  literature: obj({ records: arr(any), cached: bool }),
  'llm.test': obj(),
  'lens.asset': obj({ ok: bool }),
  'prediction.evaluate': obj({ ok: bool }),
  'prediction.seal': obj({ ok: bool }),
  'prediction.assert': obj({ ok: bool }),
  'messages.clear': obj({ deleted: num }),
  'chat.regenerate': obj({ assistant: any }),
  'chat.rewrite': obj({ message: any }),
  'messages.list': obj({
    messages: arr(obj({ id: str, role: str, content: str })),
  }),
  'jobs.list': obj({
    jobs: arr(
      obj(
        {
          id: str,
          session_id: str,
          status: jobStatus,
          job_type: str,
          params_json: any,
          result_json: any,
          created_at: str,
          completed_at: str,
          compiler_used: str,
          fallback_estimate: bool,
        },
        ['id', 'session_id', 'status', 'job_type', 'params_json', 'created_at'],
      ),
    ),
  }),
  job: obj({
    job: obj(
      {
        id: str,
        session_id: str,
        status: jobStatus,
        job_type: str,
        params_json: any,
        result_json: any,
        created_at: str,
        completed_at: str,
        compiler_used: str,
        fallback_estimate: bool,
      },
      ['id', 'session_id', 'status', 'job_type', 'params_json', 'created_at'],
    ),
  }),
  radar: obj({
    radar_endpoints: arr(any),
  }),
  'notebooks.list': obj({
    notebooks: arr(any),
  }),
  'agents.system': obj({
    roles: arr(obj({ role: str, description: str })),
  }),
  'agents.custom': obj({
    agents: arr(any),
  }),
  'labs.list': obj({
    tenants: arr(any),
  }),
  'obligations.close': obj({
    ok: bool,
    id: str,
    closed: bool,
    reasons: arr(str),
    already_closed: bool,
  }, ['ok']),
  'quantum.cancel': obj({
    ok: bool,
    job_id: str,
    status: str,
    provider_recall_attempted: bool,
    provider_recall_status: str,
  }, ['ok']),
  'agents.custom.delete': obj({
    ok: bool,
    agent_id: str,
  }),
  'metasearch.change': obj({
    success: bool,
    proposal: any,
    message: str,
  }, ['success']),
  'settings.update': obj({
    status: str,
    settings: any,
  }),
  'canvas.saved': obj({
    status: str,
    session_id: str,
  }),
  'chat.feedback': obj({
    recorded: bool,
    message_id: str,
    vote: str,
  }),
  'loop.action': obj({
    loop_id: str,
    action: str,
    status: str,
  }),
  'hf.cancel': obj({
    ok: bool,
    task_id: str,
    status: str,
  }),
  'vault.lease.revoke': obj({
    lease_id: str,
    revoked: bool,
  }),
  'quantum.canary': obj({
    ok: bool,
    results: any,
    cached: bool,
    cache_ttl_seconds: num,
  }, ['ok']),
  'tenant.write': obj({
    id: str,
    display_name: str,
    autonomy_level: str,
    status: str,
    aborted_jobs_count: num,
  }, ['id']),
  'session.refund': obj({
    id: str,
    current_spend_usd: num,
  }, ['id']),
  'cost.summary': obj({
    cap_usd: num,
    spent_usd: num,
    remaining_usd: { ...num, nullable: true },
    active_provider: str,
    currency: str,
  }),
  'memory.recall': obj({
    snippets: arr(any),
    tenant: str,
  }),
  'workflow.plan': obj({
    ok: bool,
    dry_run: bool,
    workflow_id: str,
    steps: arr(any),
    unresolved_references: arr(any),
    compensations_planned: arr(any),
  }),
  'workflow.run': obj({
    ok: bool,
    workflow_id: str,
    steps_completed: num,
    step_records: arr(any),
    spent_usd: num,
    last_results: obj(),
    compensations: arr(any),
  }),
  'qir.compile': obj({
    num_qubits: num,
    depth: num,
    gate_count: num,
    t_count: num,
    qir_profile: str,
  }),
  'session.context': obj({
    session_id: str,
    turns: num,
    turn_cap: num,
    history_chars: num,
    history_chars_cap: num,
    would_drop: num,
    pressure_pct: num,
    prompt_tokens: num,
    completion_tokens: num,
  }),
  'auth.challenge': obj({
    nonce: str,
    message: str,
    expires_at: str,
  }),
  'auth.whoami': obj({
    authenticated: bool,
  }),
  'quantum.backends': obj({
    backends: arr(
      obj({
        provider: str,
        display_name: str,
        modality: str,
        device: str,
        credential_state: str,
        telemetry: str,
        executable_now: bool,
      }),
    ),
  }),
} satisfies Record<string, ResponseSchema>;

export type ContractName = keyof typeof RESPONSE_SCHEMAS;

export interface ContractIssue {
  path: string;
  message: string;
}

function validate(value: unknown, schema: ResponseSchema, path: string): ContractIssue[] {
  if (schema.type === 'any') return [];
  if (value === null || value === undefined) {
    return schema.nullable ? [] : [{ path, message: `expected ${schema.type}, got ${value === null ? 'null' : 'undefined'}` }];
  }
  switch (schema.type) {
    case 'union': {
      const members = schema.anyOf ?? [];
      if (members.some((member) => validate(value, member, path).length === 0)) return [];
      return [{ path, message: `expected one of ${members.map((member) => member.type).join(', ')}` }];
    }
    case 'string':
      return typeof value === 'string' ? [] : [{ path, message: `expected string, got ${typeof value}` }];
    case 'number':
      return typeof value === 'number' && Number.isFinite(value)
        ? []
        : [{ path, message: `expected finite number, got ${String(value)}` }];
    case 'boolean':
      return typeof value === 'boolean' ? [] : [{ path, message: `expected boolean, got ${typeof value}` }];
    case 'array': {
      if (!Array.isArray(value)) return [{ path, message: `expected array, got ${typeof value}` }];
      if (!schema.items) return [];
      return value.flatMap((entry, index) => validate(entry, schema.items as ResponseSchema, `${path}[${index}]`));
    }
    case 'object': {
      if (typeof value !== 'object' || value === null || Array.isArray(value)) {
        return [{ path, message: `expected object, got ${Array.isArray(value) ? 'array' : typeof value}` }];
      }
      const record = value as Record<string, unknown>;
      const issues: ContractIssue[] = [];
      for (const key of schema.required ?? []) {
        if (!(key in record)) issues.push({ path: `${path}.${key}`, message: 'missing required field' });
      }
      for (const [key, child] of Object.entries(schema.properties ?? {})) {
        if (key in record) issues.push(...validate(record[key], child, `${path}.${key}`));
      }
      return issues;
    }
  }
}

export function validateResponse(name: ContractName, value: unknown): ContractIssue[] {
  return validate(value, RESPONSE_SCHEMAS[name], name);
}

const reported = new Set<ContractName>();
let driftHandler: ((name: ContractName, issues: ContractIssue[]) => void) | null = null;

export function setContractDriftHandler(
  handler: ((name: ContractName, issues: ContractIssue[]) => void) | null,
): void {
  driftHandler = handler;
}

export function strictResponseContracts(): boolean {
  return process.env.NEXT_PUBLIC_KUBONA_STRICT_CONTRACTS === '1';
}

export function reportResponseContract(name: ContractName, value: unknown): ContractIssue[] {
  const issues = validateResponse(name, value);
  if (issues.length > 0 && !reported.has(name)) {
    reported.add(name);
    const detail = issues.map((i) => `${i.path}: ${i.message}`).join('; ');
    console.error(`[response-contract] ${name} drifted: ${detail}`);
    driftHandler?.(name, issues);
  }
  return issues;
}

export function resetResponseContractReports(): void {
  reported.clear();
}

export function contract<T>(name: ContractName, value: T): T {
  const issues = reportResponseContract(name, value);
  if (issues.length > 0 && strictResponseContracts()) {
    throw new Error(`[response-contract] ${name} drifted: ${issues.map((i) => `${i.path}: ${i.message}`).join('; ')}`);
  }
  return value;
}
