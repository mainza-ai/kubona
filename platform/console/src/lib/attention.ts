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

export type DivBand = 'normal' | 'notable' | 'anomalous';

/** Standardized surprise bands (plan §8.2). Single source of truth for the
 *  legend, the z-badge, and the terrain gap-tint shader (which mirrors these
 *  cutoffs in GLSL — see SpatialCanvas3D vsupMaterial). */
export function divergenceBand(absZ: number): DivBand {
  if (!Number.isFinite(absZ)) return 'normal';
  if (absZ > 3) return 'anomalous';
  if (absZ >= 1) return 'notable';
  return 'normal';
}

export interface CollapseKey {
  source: string;
  level: string;
}

/** Duplicate collapse (R-11): same source+level inside the window refreshes
 *  the visible alert instead of stacking — one diegetic anomaly object per
 *  underlying condition, with a duplicate count. */
export function shouldCollapse(
  prev: { key: CollapseKey; at: number } | null,
  next: CollapseKey,
  now: number,
  windowMs = 5 * 60 * 1000,
): boolean {
  if (!prev) return false;
  return prev.key.source === next.source && prev.key.level === next.level && now - prev.at < windowMs;
}

export interface GateRequest {
  id: string;
  kind: string;
  label: string;
  at: number;
}

export interface GateBatch {
  kind: string;
  label: string;
  count: number;
  newestAt: number;
  ids: string[];
}

/** Gate batching (R-11): simultaneous approvals of the same kind collapse
 * into one digest entry instead of N cards. Expired requests fall out. */
export function batchGateRequests(reqs: GateRequest[], windowMs: number, now: number): GateBatch[] {
  const groups = new Map<string, GateRequest[]>();
  for (const r of reqs) {
    if (now - r.at < 0 || now - r.at > windowMs) continue;
    const g = groups.get(r.kind);
    if (g) g.push(r);
    else groups.set(r.kind, [r]);
  }
  return [...groups.entries()].map(([kind, items]) => ({
    kind,
    label: items[0].label,
    count: items.length,
    newestAt: Math.max(...items.map((i) => i.at)),
    ids: items.map((i) => i.id),
  }));
}

export interface PrecisionTracker {
  acted: Record<string, number>;
  dismissed: Record<string, number>;
}

/** Alarm precision tracking (R-11): fraction of alerts the human acted on,
 * per source. Below 0.8 on enough samples the director tightens that
 * source's thresholds instead of crying wolf. */
export function createPrecisionTracker(): PrecisionTracker {
  return { acted: {}, dismissed: {} };
}

export function recordOutcome(t: PrecisionTracker, source: string, acted: boolean): void {
  const bucket = acted ? t.acted : t.dismissed;
  bucket[source] = (bucket[source] ?? 0) + 1;
}

export function precisionFor(t: PrecisionTracker, source: string): number | null {
  const a = t.acted[source] ?? 0;
  const d = t.dismissed[source] ?? 0;
  const n = a + d;
  if (n < 5) return null;
  return a / n;
}

/** True when a source earned tighter thresholds: precision below 0.8
 * on at least 5 rated alerts. */
export function shouldTighten(t: PrecisionTracker, source: string): boolean {
  const p = precisionFor(t, source);
  return p != null && p < 0.8;
}
