import { describe, it, expect } from 'vitest';
import { BROWSER_RESERVED_UNSHIFTED, SHORTCUTS, SHORTCUT_CHORDS, SHORTCUT_TARGETS, shortcutFromEvent } from './shortcuts';

// Drift-parity test: the keyboard handler (page.tsx navigates via
// KEY_TO_SHORTCUT + SHORTCUT_TARGETS) and the palette labels must agree.
// Change a shortcut, add a branch, or add a palette entry — update the
// pinned literals below AND the other side, or this file fails.
//
// Migrated from react-test-renderer (shortcuts.test.tsx): the pinned tables
// and event parsing are pure logic and live here without a renderer; the
// palette-label parity half renders the real palette in
// shortcuts.component.test.tsx.

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
});
