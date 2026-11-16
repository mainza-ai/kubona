'use client';

import { useCallback, useEffect, useState } from 'react';
import {
  fetchNotificationPreferences,
  fetchNotifications,
  markNotifications,
  saveNotificationPreferences,
  type NotificationPreferences,
  type NotificationRow,
} from '@/lib/api';
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

  const [prefs, setPrefs] = useState<NotificationPreferences>({ quiet: false, muted_kinds: [] });

  const { data, status, error, refresh, setPollMs } = useSharedResource<NotificationRow[]>(
    `notifications:${sessionId ?? 'none'}`,
    load,
    { intervalMs: 15000 },
  );

  useEffect(() => {
    setPollMs(sessionId ? 15000 : 0);
    if (sessionId) refresh();
  }, [sessionId, refresh, setPollMs]);

  useEffect(() => {
    if (!sessionId) return;
    let live = true;
    fetchNotificationPreferences(sessionId).then((next) => {
      if (live) setPrefs(next);
    });
    return () => {
      live = false;
    };
  }, [sessionId, refresh]);

  useEffect(() => subscribeInvalidation(['notifications'], refresh), [refresh]);

  const markRead = useCallback(
    async (keys: string[]) => {
      if (!sessionId) return;
      await markNotifications(sessionId, keys, 'read');
      refresh();
    },
    [sessionId, refresh],
  );

  const updatePrefs = useCallback(
    async (next: NotificationPreferences) => {
      if (!sessionId) return;
      const saved = await saveNotificationPreferences(sessionId, next.quiet, next.muted_kinds);
      if (saved) {
        setPrefs(saved);
        refresh();
      }
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

  return { notifications: data ?? [], prefs, status, error, refresh, markRead, resolve, updatePrefs };
}
