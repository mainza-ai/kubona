'use client';

import { useCallback, useEffect } from 'react';
import { fetchNotifications, markNotifications, type NotificationRow } from '@/lib/api';
import { subscribeInvalidation } from './invalidation';
import { useSharedResource } from './shared-resource';

/** Server-owned notification inbox for one session. Polls as a fallback and
 * refreshes immediately on `notifications` SSE invalidation. */
export function useNotificationsProjection(sessionId: string | null) {
  const load = useCallback(async (): Promise<NotificationRow[]> => {
    if (!sessionId) return [];
    const payload = await fetchNotifications(sessionId);
    return payload?.notifications ?? [];
  }, [sessionId]);

  const { data, status, error, refresh, setPollMs } = useSharedResource<NotificationRow[]>(
    `notifications:${sessionId ?? 'none'}`,
    load,
    { intervalMs: 15000 },
  );

  useEffect(() => {
    setPollMs(sessionId ? 15000 : 0);
    if (sessionId) refresh();
  }, [sessionId, refresh, setPollMs]);

  useEffect(() => subscribeInvalidation(['notifications'], refresh), [refresh]);

  const markRead = useCallback(
    async (keys: string[]) => {
      if (!sessionId) return;
      await markNotifications(sessionId, keys, 'read');
      refresh();
    },
    [sessionId, refresh],
  );

  const resolve = useCallback(
    async (key: string) => {
      if (!sessionId) return;
      await markNotifications(sessionId, [key], 'resolve');
      refresh();
    },
    [sessionId, refresh],
  );

  return { notifications: data ?? [], status, error, refresh, markRead, resolve };
}
