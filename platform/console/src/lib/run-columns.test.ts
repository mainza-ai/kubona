import { describe, expect, it } from 'vitest';
import { activePart, columnsOf } from './run-columns';
import type { LoopPart } from '@/types';

function part(overrides: Partial<LoopPart> & { part_id: string; began_ms: number }): LoopPart {
  return {
    loop_id: 'loop-1',
    helper: 'lead',
    title: 'part',
    state: 'done',
    ...overrides,
  };
}

describe('run columns (C1)', () => {
  it('groups by helper in vocabulary order and keeps each column time-ordered', () => {
    const columns = columnsOf([
      part({ part_id: 'b', began_ms: 20, helper: 'quantum_job', title: 'job' }),
      part({ part_id: 'a', began_ms: 10, helper: 'lead', title: 'step 1' }),
      part({ part_id: 'c', began_ms: 30, helper: 'lead', title: 'step 2' }),
      part({ part_id: 'd', began_ms: 5, helper: 'literature', title: 'paper' }),
    ]);
    expect(columns.map((column) => column.helper)).toEqual(['lead', 'quantum_job', 'literature']);
    expect(columns[0].parts.map((p) => p.title)).toEqual(['step 1', 'step 2']);
    expect(columns[1].parts[0].title).toBe('job');
  });

  it('puts unknown helpers last in first-seen order instead of dropping them', () => {
    const columns = columnsOf([
      part({ part_id: 'x', began_ms: 1, helper: 'mystery' }),
      part({ part_id: 'y', began_ms: 2, helper: 'lead' }),
      part({ part_id: 'z', began_ms: 3, helper: 'another' }),
    ]);
    expect(columns.map((column) => column.helper)).toEqual(['lead', 'mystery', 'another']);
  });

  it('marks the running part, then the first waiting part, as active', () => {
    const running = [
      part({ part_id: 'a', began_ms: 1, state: 'done' }),
      part({ part_id: 'b', began_ms: 2, state: 'running' }),
    ];
    expect(activePart(running)?.part_id).toBe('b');

    const waiting = [
      part({ part_id: 'a', began_ms: 1, state: 'done' }),
      part({ part_id: 'b', began_ms: 2, state: 'waiting' }),
    ];
    expect(activePart(waiting)?.part_id).toBe('b');

    expect(activePart([part({ part_id: 'a', began_ms: 1, state: 'done' })])).toBeNull();
  });
});
