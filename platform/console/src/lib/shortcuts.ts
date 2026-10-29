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
  emergencyAbort: 'abort',
  help: 'help',
  dismiss: 'dismiss',
};

/**
 * Normalized KeyboardEvent.key (lowercased) -> SHORTCUTS key name.
 * Digits are meta-only (see page handler: Ctrl+digits must keep driving
 * browser tab switching on non-macOS); letters accept Cmd or Ctrl.
 */
export const KEY_TO_SHORTCUT: Record<string, ShortcutKey> = {
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
