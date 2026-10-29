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

// Wave-3 hygiene note: isCorroboratedVerdict was a one-line wrapper around
// classifyTribunalVerdict(…) === 'corroborated' with zero repo-wide callers
// (EpistemicTribunalRoom calls classifyTribunalVerdict directly for seat
// filling + quorum counting). Wiring it would mean touching
// EpistemicTribunalRoom, owned by another surface workstream, so the dead
// export is deleted here instead of threaded through. Re-add as the
// one-liner above if a caller ever needs the boolean form.
