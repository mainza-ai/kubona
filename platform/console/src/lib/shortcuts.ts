/**
 * Single shortcut const: page surfaces, CommandPalette items, and
 * KeyboardShortcutsModal rows all render from here so labels cannot drift.
 *
 * Every entry below is implemented in the shell key handler (page.tsx) and
 * labelled in the palette + modal. `overview` was removed 2026-10-29: it
 * had no navigation destination (the arena orbital view is the default),
 * so advertising ⌘0 was a dead end — advertised == implemented.
 */
export const SHORTCUTS = {
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

export type ShortcutKey = keyof typeof SHORTCUTS;

/**
 * Navigation target for a shortcut: the palette item id / onSelectSurface
 * id the shell navigates to. Single dispatch table so the keyboard handler
 * (page.tsx) and the palette (CommandPalette.tsx) cannot drift apart.
 * Targets 'manuscripts' | 'circuits' | 'arena-orbital' | 'arena-proof-dag'
 * are palette-only deep links (no shortcut key) and are intentionally
 * absent here — the parity test asserts exactly this shape.
 */
export const SHORTCUT_TARGETS: Record<ShortcutKey, string> = {
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
  experimentLab: 'experiment-lab',
  emergencyAbort: 'abort',
  help: 'help',
  dismiss: 'dismiss',
};

/**
 * Unshifted Cmd/Ctrl chords browsers keep for themselves. preventDefault
 * does not win. A shell shortcut on one of these keys must use Shift.
 */
export const BROWSER_RESERVED_UNSHIFTED = new Set([
  't', 'n', 'w', 'l', 'q', 'r', 'p', 'd', 'o', 's', 'i', 'h', 'a',
]);

export type ShortcutChord = { key: string; shift: boolean };

/** dismiss is Escape, handled by the overlay that is open, not this table. */
export const SHORTCUT_CHORDS: Record<Exclude<ShortcutKey, 'dismiss'>, ShortcutChord> = {
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
};

export function shortcutFromEvent(event: { key: string; shiftKey: boolean }): Exclude<ShortcutKey, 'dismiss'> | null {
  const key = event.key.length === 1 ? event.key.toLowerCase() : event.key;
  for (const [name, chord] of Object.entries(SHORTCUT_CHORDS) as Array<[Exclude<ShortcutKey, 'dismiss'>, ShortcutChord]>) {
    if (chord.key === key && chord.shift === event.shiftKey) return name;
  }
  return null;
}
