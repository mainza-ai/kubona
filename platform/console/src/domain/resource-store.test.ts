import { afterEach, describe, expect, it, vi } from 'vitest';
import { StopPolling, createPollingStore } from './resource-store';
import { ApiFailureError } from '@/lib/api-errors';

describe('createPollingStore', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('polls serially, publishes data, and stops without subscribers', async () => {
    vi.useFakeTimers();
    const load = vi.fn(async () => ({ n: 1 }));
    const store = createPollingStore(load, { intervalMs: 1000 });
    const listener = vi.fn();
    const unsubscribe = store.subscribe(listener);
    await vi.advanceTimersByTimeAsync(0);
    expect(load).toHaveBeenCalledTimes(1);
    expect(store.getSnapshot().status).toBe('ready');
    expect(store.getSnapshot().data).toEqual({ n: 1 });

    await vi.advanceTimersByTimeAsync(999);
    expect(load).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(load).toHaveBeenCalledTimes(2);

    unsubscribe();
    await vi.advanceTimersByTimeAsync(5000);
    expect(load).toHaveBeenCalledTimes(2);
    expect(listener).toHaveBeenCalled();
  });

  it('stops on a terminal value and on an explicit stop', async () => {
    vi.useFakeTimers();
    const load = vi.fn(async () => ({ done: true }));
    const store = createPollingStore(load, { intervalMs: 1000, shouldContinue: (data) => !data.done });
    const unsubscribe = store.subscribe(() => {});
    await vi.advanceTimersByTimeAsync(5000);
    expect(load).toHaveBeenCalledTimes(1);
    expect(store.getSnapshot().status).toBe('ready');

    const stopped = vi.fn(async () => { throw new StopPolling('gone'); });
    const second = createPollingStore(stopped, { intervalMs: 1000 });
    const off = second.subscribe(() => {});
    await vi.advanceTimersByTimeAsync(5000);
    expect(stopped).toHaveBeenCalledTimes(1);
    expect(second.getSnapshot().status).toBe('refused');
    off();
    unsubscribe();
  });

  it('keeps a structured failure so auth and transport are distinguishable', async () => {
    vi.useFakeTimers();
    const load = vi.fn(async () => {
      throw new ApiFailureError({ path: '/api/loops', status: 401, kind: 'auth', message: 'login required', at: 1 });
    });
    const store = createPollingStore(load, { intervalMs: 1000 });
    const off = store.subscribe(() => {});
    await vi.advanceTimersByTimeAsync(0);
    expect(store.getSnapshot().status).toBe('refused');
    expect(store.getSnapshot().failure?.kind).toBe('auth');
    off();
  });

  it('keeps the previous snapshot stale and backs off on failure', async () => {
    vi.useFakeTimers();
    const load = vi
      .fn<() => Promise<{ n: number }>>()
      .mockResolvedValueOnce({ n: 7 })
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue({ n: 8 });
    const store = createPollingStore(load, { intervalMs: 1000, maxBackoffMs: 2000 });
    const unsubscribe = store.subscribe(() => {});
    await vi.advanceTimersByTimeAsync(0);
    expect(store.getSnapshot().data).toEqual({ n: 7 });

    await vi.advanceTimersByTimeAsync(1000);
    expect(store.getSnapshot().status).toBe('stale');
    expect(store.getSnapshot().data).toEqual({ n: 7 });
    expect(store.getSnapshot().error).toContain('offline');
    expect(store.getSnapshot().intervalMs).toBe(1500);

    await vi.advanceTimersByTimeAsync(1500);
    expect(store.getSnapshot().status).toBe('ready');
    expect(store.getSnapshot().data).toEqual({ n: 8 });
    expect(store.getSnapshot().intervalMs).toBe(1000);
    unsubscribe();
  });

  it('queues a follow-up load when refresh() arrives while an initial load is in flight', async () => {
    vi.useFakeTimers();
    let resolveFirst!: (value: { n: number }) => void;
    const first = new Promise<{ n: number }>((resolve) => {
      resolveFirst = resolve;
    });
    const load = vi
      .fn<() => Promise<{ n: number }>>()
      .mockReturnValueOnce(first)
      .mockResolvedValueOnce({ n: 2 });
    const store = createPollingStore(load, { intervalMs: 5000 });
    const unsubscribe = store.subscribe(() => {});
    expect(load).toHaveBeenCalledTimes(1);

    // An SSE invalidation arrives before the boot fetch finishes.
    store.refresh();
    expect(load).toHaveBeenCalledTimes(1);

    resolveFirst({ n: 1 });
    await vi.advanceTimersByTimeAsync(0);
    expect(load).toHaveBeenCalledTimes(2);
    expect(store.getSnapshot().data).toEqual({ n: 2 });
    unsubscribe();
  });
});
