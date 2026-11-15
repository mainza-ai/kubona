import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { BROWSER_RESERVED_UNSHIFTED, SHORTCUTS, SHORTCUT_CHORDS, SHORTCUT_TARGETS, shortcutFromEvent } from './shortcuts';
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
  laboratories: '⌘⇧L',
  sovereignAirGap: '⌘⇧S',
  tribunalRoom: '⌘⇧T',
  commandPalette: '⌘K',
  copilot: '⌘J',
  experimentLab: '⌘E',
  about: '⌘⇧I',
  newCampaign: '⌘⇧N',
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
  experimentLab: 'experiment-lab',
  about: 'about',
  newCampaign: 'new',
  emergencyAbort: 'abort',
  help: 'help',
  dismiss: 'dismiss',
};

const PINNED_CHORDS = {
  livingCanvas: { key: '1', shift: false },
  flightRecorder: { key: '2', shift: false },
  epistemicOrb: { key: '3', shift: false },
  livingNotebook: { key: '4', shift: false },
  sentinelGuardrails: { key: '5', shift: false },
  fleetRadar: { key: '6', shift: false },
  cognitiveMesh: { key: '7', shift: false },
  previewGallery: { key: '8', shift: false },
  huggingFaceHub: { key: '9', shift: false },
  laboratories: { key: 'l', shift: true },
  sovereignAirGap: { key: 's', shift: true },
  tribunalRoom: { key: 't', shift: true },
  commandPalette: { key: 'k', shift: false },
  copilot: { key: 'j', shift: false },
  experimentLab: { key: 'e', shift: false },
  about: { key: 'i', shift: true },
  newCampaign: { key: 'n', shift: true },
  emergencyAbort: { key: '.', shift: false },
  help: { key: '?', shift: false },
} as const;

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

  it('pins every handler chord explicitly', () => {
    expect(SHORTCUT_CHORDS).toEqual(PINNED_CHORDS);
  });

  it('refuses an unshifted browser-reserved chord', () => {
    for (const [name, chord] of Object.entries(SHORTCUT_CHORDS)) {
      if (!chord.shift && BROWSER_RESERVED_UNSHIFTED.has(chord.key)) {
        throw new Error(`${name} binds ⌘${chord.key.toUpperCase()}, which the browser keeps`);
      }
    }
    expect(shortcutFromEvent({ key: 't', shiftKey: false })).toBeNull();
    expect(shortcutFromEvent({ key: 'T', shiftKey: true })).toBe('tribunalRoom');
    expect(shortcutFromEvent({ key: 'n', shiftKey: false })).toBeNull();
    expect(shortcutFromEvent({ key: 'l', shiftKey: true })).toBe('laboratories');
  });

  it('every palette shortcut glyph has a handler branch', () => {
    const glyphToName = new Map<string, string>(
      Object.entries(SHORTCUTS).map(([name, glyph]) => [glyph as string, name])
    );
    for (const glyph of paletteShortcutGlyphs()) {
      const name = glyphToName.get(glyph);
      expect(name, `palette advertises ${glyph} with no SHORTCUTS entry`).toBeDefined();
      const chord = SHORTCUT_CHORDS[name as keyof typeof SHORTCUT_CHORDS];
      expect(chord, `palette advertises ${glyph} with no handler chord`).toBeDefined();
      expect(shortcutFromEvent({ key: chord.key, shiftKey: chord.shift })).toBe(name);
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
