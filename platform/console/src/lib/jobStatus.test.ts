import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { jobStatusLabel } from './api';

// W2.9 drift-proof: the label map must match the proto JobStatus enum
// exactly. A new enum value without a label fails here instead of
// rendering as a raw number (or worse, a wrong label).
describe('jobStatusLabel parity with proto JobStatus', () => {
  it('covers every enum value with the canonical label', () => {
    const here = dirname(fileURLToPath(import.meta.url));
    const proto = readFileSync(join(here, '..', '..', '..', 'bridge', 'proto', 'kubona.proto'), 'utf8');
    const enumBody = proto.slice(proto.indexOf('enum JobStatus'), proto.indexOf('}', proto.indexOf('enum JobStatus')));
    const entries = [...enumBody.matchAll(/(\w+)\s*=\s*(\d+)/g)].map((m) => ({
      name: m[1].replace(/^JOB_STATUS_/, ''),
      num: Number(m[2]),
    }));
    expect(entries.length).toBeGreaterThan(0);
    for (const { name, num } of entries) {
      expect(jobStatusLabel(num)).toBe(name);
      expect(jobStatusLabel(`JOB_STATUS_${name}`)).toBe(name);
    }
  });

  it('passes unknowns through verbatim, never invents', () => {
    expect(jobStatusLabel(99)).toBe('99');
    expect(jobStatusLabel('SOMETHING_ELSE')).toBe('SOMETHING_ELSE');
    expect(jobStatusLabel(undefined)).toBe('UNKNOWN');
  });
});
