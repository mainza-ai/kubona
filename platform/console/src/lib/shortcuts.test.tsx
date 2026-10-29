import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { SHORTCUTS, SHORTCUT_TARGETS, KEY_TO_SHORTCUT } from './shortcuts';
import { CommandPalette } from '@/components/CommandPalette';

// Drift-parity test: the keyboard handler (page.tsx navigates via
// KEY_TO_SHORTCUT + SHORTCUT_TARGETS) and the palette labels must agree.
// Change a shortcut, add a branch, or add a palette entry — update the
// pinned literals below AND the other side, or this file fails.

const PINNED_SHORTCUTS = {
  livingCanvas: '⌘1',
  flightRecorder: '⌘2',
  epistemicOrb: '⌘3',
  livingNotebook: '⌘4',
  sentinelGuardrails: '⌘5',
  fleetRadar: '⌘6',
  cognitiveMesh: '⌘7',
  previewGallery: '⌘8',
  huggingFaceHub: '⌘9',
  laboratories: '⌘L',
  sovereignAirGap: '⌘S',
  tribunalRoom: '⌘T',
  commandPalette: '⌘K',
  copilot: '⌘J',
  about: '⌘I',
  newCampaign: '⌘N',
  emergencyAbort: '⌘.',
  help: '?',
  dismiss: 'Esc',
} as const;

const PINNED_TARGETS: Record<string, string> = {
  livingCanvas: 'arena',
  flightRecorder: 'recorder',
  epistemicOrb: 'manifold',
  livingNotebook: 'proofs',
  sentinelGuardrails: 'guardrails',
  fleetRadar: 'fleet',
  cognitiveMesh: 'mesh',
  previewGallery: 'gallery',
  huggingFaceHub: 'models',
  laboratories: 'labs',
  sovereignAirGap: 'settings',
  tribunalRoom: 'tribunal',
  commandPalette: 'palette',
  copilot: 'copilot',
  about: 'about',
  newCampaign: 'new',
  emergencyAbort: 'abort',
  help: 'help',
  dismiss: 'dismiss',
};

const PINNED_KEYS: Record<string, string> = {
  '1': 'livingCanvas',
  '2': 'flightRecorder',
  '3': 'epistemicOrb',
  '4': 'livingNotebook',
  '5': 'sentinelGuardrails',
  '6': 'fleetRadar',
  '7': 'cognitiveMesh',
  '8': 'previewGallery',
  '9': 'huggingFaceHub',
  l: 'laboratories',
  s: 'sovereignAirGap',
  t: 'tribunalRoom',
  k: 'commandPalette',
  j: 'copilot',
  i: 'about',
  n: 'newCampaign',
  '.': 'emergencyAbort',
  '?': 'help',
};

function paletteShortcutGlyphs(): string[] {
  let tree: TestRenderer.ReactTestRenderer;
  act(() => {
    tree = TestRenderer.create(
      <CommandPalette
        isOpen={true}
        onClose={vi.fn()}
        onSelectSurface={vi.fn()}
        onNewCampaign={vi.fn()}
        onEmergencyAbort={vi.fn()}
        onOpenAbout={vi.fn()}
        onToggleCopilot={vi.fn()}
        onOpenShortcuts={vi.fn()}
      />
    );
  });
  return tree!.root
    .findAllByType('kbd')
    .map((k) => (k.children ?? []).map((c) => String(c)).join(''))
    .filter((g) => g !== 'Esc');
}

describe('shortcuts single source of truth', () => {
  it('pins every SHORTCUTS glyph explicitly', () => {
    expect(SHORTCUTS).toEqual(PINNED_SHORTCUTS);
  });

  it('pins every shortcut target explicitly (handler branches)', () => {
    expect(SHORTCUT_TARGETS).toEqual(PINNED_TARGETS);
  });

  it('pins every handler key explicitly', () => {
    expect(KEY_TO_SHORTCUT).toEqual(PINNED_KEYS);
  });

  it('every palette shortcut glyph has a handler branch', () => {
    const glyphToName = new Map<string, string>(
      Object.entries(SHORTCUTS).map(([name, glyph]) => [glyph as string, name])
    );
    for (const glyph of paletteShortcutGlyphs()) {
      const name = glyphToName.get(glyph);
      expect(name, `palette advertises ${glyph} with no SHORTCUTS entry`).toBeDefined();
      expect(KEY_TO_SHORTCUT[glyph.replace('⌘', '').toLowerCase()], `palette advertises ${glyph} with no handler branch`).toBeDefined();
      expect(SHORTCUT_TARGETS[name as keyof typeof SHORTCUTS], `palette advertises ${glyph} with no handler target`).toBeDefined();
    }
  });

  it('every implemented shortcut has a palette label', () => {
    const glyphs = new Set(paletteShortcutGlyphs());
    for (const [name, glyph] of Object.entries(SHORTCUTS)) {
      if (name === 'dismiss') continue; // Esc is per-component chrome, not a shell branch
      expect(glyphs.has(glyph as string), `handler implements ${name} (${glyph}) with no palette label`).toBe(true);
    }
  });
});
