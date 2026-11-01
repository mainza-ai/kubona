import { afterEach, describe, expect, it, vi } from 'vitest';
import { createPollingStore } from './resource-store';

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
});
