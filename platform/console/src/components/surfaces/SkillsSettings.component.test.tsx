import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import type { SkillsCatalog } from '@/lib/api';
import { SkillsSettings } from './SkillsSettings';

const catalog: SkillsCatalog = {
  skills: [
    {
      name: 'pes-scan',
      description: 'Plan a potential-energy-surface scan with a live classical anchor.',
      role: 'copilot',
      tools: ['get_anchor', 'submit_local_sim'],
      source: 'seed',
      enabled: true,
    },
    {
      name: 'lab-check',
      description: 'A lab-owned readiness check.',
      role: 'copilot',
      tools: [],
      source: 'user',
      enabled: true,
    },
  ],
  faults: [{ code: 'skill_symlink', skill: 'linked', detail: 'SKILL.md is a symlink; refused' }],
  limits: { max_skills: 64, max_skill_bytes: 16384, max_description: 200 },
};

describe('SkillsSettings in a real browser', () => {
  it('lists names, descriptions, tools and rejected-file faults', async () => {
    const screen = await render(<SkillsSettings load={async () => catalog} />);

    await expect.element(screen.getByText('pes-scan')).toBeVisible();
    await expect.element(screen.getByText(/Plan a potential-energy-surface scan/)).toBeVisible();
    await expect.element(screen.getByText('submit_local_sim')).toBeVisible();
    await expect.element(screen.getByText('lab-check')).toBeVisible();
    await expect.element(screen.getByText(/skill_symlink/)).toBeVisible();
    await expect.element(screen.getByText(/SKILL\.md is a symlink/)).toBeVisible();
    await expect.element(screen.getByText(/2 skills · limit 64/)).toBeVisible();
  });

  it('refuses visibly when the catalogue is unavailable', async () => {
    const screen = await render(<SkillsSettings load={async () => null} />);
    await expect.element(screen.getByText(/Skill catalogue unavailable/)).toBeVisible();
    await expect.element(screen.getByText(/No skills are advertised/)).toBeVisible();
  });
});
