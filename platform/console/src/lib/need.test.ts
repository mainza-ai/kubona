import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import {
  NEED_KINDS,
  isLiveNeed,
  isNeed,
  isNeedKind,
  needSentence,
  remedyFor,
} from './need';

// Single source: the Go registry and this map both mirror the contract file.
// Reading it here makes a fork (kind added on one side only) a red test.
function contract(): {
  kinds: Record<string, { sentence: string; remedy: { id: string; label: string; surface: string; section?: string } }>;
} {
  const raw = fs.readFileSync(path.resolve(process.cwd(), '..', 'contracts', 'need-kinds.json'), 'utf8');
  return JSON.parse(raw);
}

describe('need vocabulary', () => {
  it('covers every contract kind and no invented ones', () => {
    const kinds = Object.keys(contract().kinds).sort();
    expect([...NEED_KINDS].sort()).toEqual(kinds);
  });

  it('pins the single remedy per kind to the contract', () => {
    for (const [kind, entry] of Object.entries(contract().kinds)) {
      const remedy = remedyFor(kind);
      expect(remedy, `kind ${kind}`).not.toBeNull();
      expect(remedy!.id).toBe(entry.remedy.id);
      expect(remedy!.label).toBe(entry.remedy.label);
      expect(remedy!.surface).toBe(entry.remedy.surface);
      expect(remedy!.section ?? '').toBe(entry.remedy.section ?? '');
    }
  });

  it('refuses unknown kinds instead of rendering a made-up remedy', () => {
    expect(isNeedKind('feel_better')).toBe(false);
    expect(remedyFor('feel_better')).toBeNull();
    expect(remedyFor('')).toBeNull();
  });
});

describe('wire need validation', () => {
  it('accepts a complete need and rejects unknown kinds or empty reasons', () => {
    expect(isNeed({ kind: 'backend_offline', reason: 'bridge down' })).toBe(true);
    expect(isNeed({ kind: 'backend_offline', reason: 'bridge down', target: 'ibm_fez', past: true })).toBe(true);
    expect(isNeed({ kind: 'not_a_kind', reason: 'x' })).toBe(false);
    expect(isNeed({ kind: 'retry', reason: '   ' })).toBe(false);
    expect(isNeed({ kind: 'retry' })).toBe(false);
    expect(isNeed(null)).toBe(false);
    expect(isNeed('retry')).toBe(false);
  });

  it('keeps the server reason verbatim and never invents a cause', () => {
    expect(needSentence({ kind: 'credentials', reason: 'provider returned 401' })).toBe('provider returned 401');
    expect(needSentence({ kind: 'credentials', reason: '  ' })).toBe('Cause not recorded.');
  });

  it('separates a live need from one that was overcome', () => {
    expect(isLiveNeed({ kind: 'retry', reason: 'timeout' })).toBe(true);
    expect(isLiveNeed({ kind: 'retry', reason: 'timeout', past: true })).toBe(false);
    expect(isLiveNeed(null)).toBe(false);
  });
});
