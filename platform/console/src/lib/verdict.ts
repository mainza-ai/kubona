/**
 * One shared tribunal verdict classifier: seat filling and quorum counting
 * must agree on what counts as corroborated/falsified, or the chamber can
 * show CLEARED while the gauge counts contested.
 */
export type TribunalVote = 'corroborated' | 'contested' | 'falsified' | 'abstain' | 'pending';

export function classifyTribunalVerdict(verdict: string | null | undefined): Exclude<TribunalVote, 'pending'> | 'contested' {
  const v = (verdict || '').toLowerCase();
  if (/corroborat|pass|confirm|verif/.test(v)) return 'corroborated';
  if (/falsif|fail|reject/.test(v)) return 'falsified';
  if (/abstain/.test(v)) return 'abstain';
  return 'contested';
}

export function isCorroboratedVerdict(verdict: string | null | undefined): boolean {
  return classifyTribunalVerdict(verdict) === 'corroborated';
}
