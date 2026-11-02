'use client';

export type InvalidationDomain = 'campaign' | 'jobs' | 'tasks' | 'obligations' | 'loops' | 'cost';

const listeners = new Set<(domains: ReadonlySet<InvalidationDomain>) => void>();

export function domainsForEventTypes(eventTypes: readonly string[]): Set<InvalidationDomain> {
  const out = new Set<InvalidationDomain>();
  for (const type of eventTypes) {
    if (type.startsWith('job_')) out.add('jobs');
    if (type.startsWith('task_')) out.add('tasks');
    if (type.startsWith('campaign_')) out.add('campaign');
    if (type.startsWith('loop_') || type === 'autoresearch_loop_reconciled') out.add('loops');
    if (type.startsWith('obligation_')) out.add('obligations');
    if (type.startsWith('llm_spend_') || type.startsWith('session_spend_')) out.add('cost');
  }
  return out;
}

export function publishInvalidation(domains: ReadonlySet<InvalidationDomain>): void {
  if (domains.size === 0) return;
  for (const listener of listeners) listener(domains);
}

export function resetInvalidationForTests(): void {
  listeners.clear();
}

export function subscribeInvalidation(
  domains: readonly InvalidationDomain[],
  listener: () => void,
): () => void {
  const wanted = new Set(domains);
  const wrapped = (hit: ReadonlySet<InvalidationDomain>) => {
    for (const domain of wanted) {
      if (hit.has(domain)) {
        listener();
        return;
      }
    }
  };
  listeners.add(wrapped);
  return () => listeners.delete(wrapped);
}
