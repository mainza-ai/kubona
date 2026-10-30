// Response contracts for the live console boundary.
//
// These are intentionally small structural schemas, not a full OpenAPI
// generator: they pin the fields the console reads and reject a payload that
// drifts underneath a typed wrapper. Extra backend fields stay legal. Drift is
// reported once per contract name and never replaces the payload — callers
// keep their existing empty/refused fallbacks.

export type SchemaType = 'object' | 'array' | 'string' | 'number' | 'boolean' | 'any';

export interface ResponseSchema {
  type: SchemaType;
  required?: string[];
  properties?: Record<string, ResponseSchema>;
  items?: ResponseSchema;
  nullable?: boolean;
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
  'messages.list': obj({
    messages: arr(obj({ id: str, role: str, content: str })),
  }),
  'jobs.list': obj({
    jobs: arr(obj({ id: str, session_id: str, status: str })),
  }),
  job: obj({
    job: obj({ id: str, session_id: str, status: str }),
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

export function reportResponseContract(name: ContractName, value: unknown): ContractIssue[] {
  const issues = validateResponse(name, value);
  if (issues.length > 0 && !reported.has(name)) {
    reported.add(name);
    console.error(`[response-contract] ${name} drifted: ${issues.map((i) => `${i.path}: ${i.message}`).join('; ')}`);
  }
  return issues;
}

export function resetResponseContractReports(): void {
  reported.clear();
}

export function contract<T>(name: ContractName, value: T): T {
  reportResponseContract(name, value);
  return value;
}
