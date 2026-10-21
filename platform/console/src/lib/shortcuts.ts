/**
 * Single shortcut const: page surfaces, CommandPalette items, and
 * KeyboardShortcutsModal rows all render from here so labels cannot drift.
 */
export const SHORTCUTS = {
  overview: '⌘0',
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
