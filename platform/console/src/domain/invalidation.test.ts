import { describe, expect, it, vi } from 'vitest';
import { domainsForEventTypes, publishInvalidation, subscribeInvalidation } from './invalidation';

describe('domain invalidation', () => {
  it('maps ledger event families to domain keys', () => {
    expect([
      ...domainsForEventTypes([
        'job_settled',
        'task_created',
        'campaign_synthesized',
        'loop_completed',
        'llm_spend_estimated',
        'llm_settings_updated',
        'llm_provider_verified',
      ]),
    ]).toEqual(expect.arrayContaining(['jobs', 'tasks', 'campaign', 'loops', 'cost', 'llm']));
    expect([...domainsForEventTypes(['chat_message_appended'])]).toEqual([]);
  });

  it('notifies only subscribers whose domain changed', () => {
    const jobs = vi.fn();
    const tasks = vi.fn();
    const offJobs = subscribeInvalidation(['jobs'], jobs);
    const offTasks = subscribeInvalidation(['tasks'], tasks);
    publishInvalidation(new Set(['jobs']));
    expect(jobs).toHaveBeenCalledTimes(1);
    expect(tasks).toHaveBeenCalledTimes(0);
    offJobs();
    offTasks();
  });
});
