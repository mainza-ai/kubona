'use client';

import { useEffect, useRef } from 'react';
import { useAttention } from '@/lib/attention-context';
import type { NotificationRow } from '@/lib/api';

/**
 * Routes unread notification severities into the Attention Director once per
 * key. Warning/danger notifications summon; informational ones are notable.
 * Quiet mode keeps the inbox but suppresses the director/ambient path.
 */
export function NotificationAttentionBridge({
  notifications,
  quiet,
}: {
  notifications: NotificationRow[];
  quiet: boolean;
}) {
  const { notify } = useAttention();
  const seen = useRef(new Set<string>());

  useEffect(() => {
    if (quiet) return;
    for (const item of notifications) {
      if (item.read_at || seen.current.has(item.key)) continue;
      seen.current.add(item.key);
      notify({
        level: item.severity === 'warning' || item.severity === 'danger' ? 'anomalous' : 'notable',
        source: item.kind,
      });
    }
  }, [notifications, quiet, notify]);

  return null;
}
