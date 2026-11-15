import { describe, expect, it } from 'vitest';
import {
  NEEDS_YOU_SEEN_CAP,
  parseSeenSignatures,
  pruneSeenSignatures,
  serializeSeenSignatures,
} from './needs-you-seen';

describe('needs-you seen state', () => {
  it('parses only string signatures and never throws on corruption', () => {
    expect([...parseSeenSignatures(null)]).toEqual([]);
    expect([...parseSeenSignatures('not json')]).toEqual([]);
    expect([...parseSeenSignatures('{"a":1}')]).toEqual([]);
    expect([...parseSeenSignatures('["ob_1@t1",2,null,"ob_2@t2",""]')]).toEqual(['ob_1@t1', 'ob_2@t2']);
  });

  it('bounds the serialized set so the store cannot grow forever', () => {
    const many = new Set(Array.from({ length: NEEDS_YOU_SEEN_CAP + 25 }, (_, index) => `ob_${index}@t`));
    const parsed = parseSeenSignatures(serializeSeenSignatures(many));
    expect(parsed.size).toBe(NEEDS_YOU_SEEN_CAP);
    // The newest signatures survive; the oldest are evicted.
    expect(parsed.has(`ob_${NEEDS_YOU_SEEN_CAP + 24}@t`)).toBe(true);
    expect(parsed.has('ob_0@t')).toBe(false);
  });

  it('keeps the set untouched while the list is still loading', () => {
    const seen = new Set(['ob_1@t']);
    expect([...pruneSeenSignatures(seen, null)]).toEqual(['ob_1@t']);
  });

  it('drops signatures that are no longer waiting, so a returning row is new again', () => {
    const seen = new Set(['ob_1@t', 'ob_2@t']);
    expect([...pruneSeenSignatures(seen, ['ob_2@t', 'ob_3@t'])]).toEqual(['ob_2@t']);
  });
});
