// Attention Director routing (P4): which sensory channel earns an event,
// and which presence modes may receive it. Pure logic — pinned by tests.
// Priority rule (plan §11): Priority = Cost of Waiting × Irreversibility.
//
// Levels: normal (L0 ambient) · notable (L1 glance) · anomalous (L3 summon).
// L2 nudge fires on optimizer stall / tribunal deadlock / 80% spend.
// Presence gating: Hands suppresses non-critical (glance+nudge collapse to
// ambient); Away drops everything below summon (explicitly flagged, never
// silently — the caller sees `suppressed: true`).

export type PresenceMode = 'hands' | 'eyes' | 'ear' | 'away';
export type AlertLevel = 'normal' | 'notable' | 'anomalous';
export type DirectorChannel = 'ambient' | 'glance' | 'nudge' | 'summon';
export type DirectorSound = 'none' | 'tick' | 'chime';

export interface DirectorInput {
  level: AlertLevel;
  source: string;
  spendUsed?: number;
  spendCap?: number;
  deadlocked?: boolean;
}

export interface DirectorRoute {
  channel: DirectorChannel;
  level: AlertLevel;
  reason: string;
  sound: DirectorSound;
  suppressed: boolean;
}

const RANK: Record<DirectorChannel, number> = { ambient: 0, glance: 1, nudge: 2, summon: 3 };

function baseRoute(input: DirectorInput): { channel: DirectorChannel; sound: DirectorSound; reason: string } {
  // Spend and deadlock escalate independently of the z-band.
  if (input.spendCap != null && input.spendCap > 0 && input.spendUsed != null) {
    const ratio = input.spendUsed / input.spendCap;
    if (ratio >= 1) {
      return { channel: 'summon', sound: 'chime', reason: `spend cap reached (${input.spendUsed.toFixed(2)}/${input.spendCap.toFixed(2)})` };
    }
    if (ratio >= 0.8) {
      return { channel: 'nudge', sound: 'tick', reason: `spend at ${(ratio * 100).toFixed(0)}% of cap` };
    }
  }
  if (input.deadlocked) {
    return { channel: 'nudge', sound: 'tick', reason: `${input.source}: deadlocked` };
  }
  switch (input.level) {
    case 'anomalous':
      return { channel: 'summon', sound: 'chime', reason: `${input.source}: anomaly (|z| > 3)` };
    case 'notable':
      return { channel: 'glance', sound: 'none', reason: `${input.source}: notable surprise (1 ≤ |z| ≤ 3)` };
    default:
      return { channel: 'ambient', sound: 'none', reason: `${input.source}: nominal` };
  }
}

/** Route one event for one presence mode. */
export function routeAlert(input: DirectorInput, presence: PresenceMode): DirectorRoute {
  const base = baseRoute(input);
  const withLevel = { ...base, level: input.level };
  if (presence === 'away' && withLevel.channel !== 'summon') {
    return { ...withLevel, channel: 'ambient', sound: 'none', suppressed: true };
  }
  if (presence === 'hands' && (withLevel.channel === 'glance' || withLevel.channel === 'nudge')) {
    return { ...withLevel, channel: 'ambient', suppressed: true };
  }
  return { ...withLevel, suppressed: false };
}

/** Pick the highest-severity route across pending inputs. */
export function routeWorst(inputs: DirectorInput[], presence: PresenceMode): DirectorRoute | null {
  let best: DirectorRoute | null = null;
  for (const input of inputs) {
    const r = routeAlert(input, presence);
    if (!best || RANK[r.channel] > RANK[best.channel]) best = r;
  }
  return best;
}
