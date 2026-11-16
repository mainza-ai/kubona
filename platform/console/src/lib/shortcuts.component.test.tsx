import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { SHORTCUTS, SHORTCUT_CHORDS, SHORTCUT_TARGETS, shortcutFromEvent } from './shortcuts';
import { CommandPalette } from '@/components/CommandPalette';

// Migrated from react-test-renderer (shortcuts.test.tsx). The old suite read
// `kbd` children off the test renderer tree; here the palette is rendered in
// Chromium and the glyphs are read from the real <kbd> elements it paints, so
// a glyph that exists in the tree but is not rendered (or vice versa) fails.
//
// Drift-parity: the keyboard handler (page.tsx navigates via
// KEY_TO_SHORTCUT + SHORTCUT_TARGETS) and the palette labels must agree.
// Change a shortcut, add a branch, or add a palette entry — update the pinned
// literals in shortcuts.test.ts AND the other side, or this file fails.

async function paletteShortcutGlyphs(): Promise<string[]> {
  const screen = await render(
    <CommandPalette
      isOpen={true}
      onClose={vi.fn()}
      onSelectSurface={vi.fn()}
      onNewCampaign={vi.fn()}
      onEmergencyAbort={vi.fn()}
      onOpenAbout={vi.fn()}
      onToggleCopilot={vi.fn()}
      onOpenShortcuts={vi.fn()}
    />,
  );
  const glyphs = [...screen.container.querySelectorAll('kbd')]
    .map((kbd) => kbd.textContent?.trim() ?? '')
    .filter((glyph) => glyph !== 'Esc');
  await screen.unmount();
  // Guard against a vacuous parity loop if the palette ever stops painting
  // its shortcut chips.
  expect(glyphs.length).toBeGreaterThan(0);
  return glyphs;
}

describe('palette labels agree with the shortcut tables', () => {
  it('every palette shortcut glyph has a handler branch', async () => {
    const glyphToName = new Map<string, string>(
      Object.entries(SHORTCUTS).map(([name, glyph]) => [glyph as string, name]),
    );
    for (const glyph of await paletteShortcutGlyphs()) {
      const name = glyphToName.get(glyph);
      expect(name, `palette advertises ${glyph} with no SHORTCUTS entry`).toBeDefined();
      const chord = SHORTCUT_CHORDS[name as keyof typeof SHORTCUT_CHORDS];
      expect(chord, `palette advertises ${glyph} with no handler chord`).toBeDefined();
      expect(shortcutFromEvent({ key: chord.key, shiftKey: chord.shift })).toBe(name);
      expect(
        SHORTCUT_TARGETS[name as keyof typeof SHORTCUTS],
        `palette advertises ${glyph} with no handler target`,
      ).toBeDefined();
    }
  });

  it('every implemented shortcut has a palette label', async () => {
    const glyphs = new Set(await paletteShortcutGlyphs());
    for (const [name, glyph] of Object.entries(SHORTCUTS)) {
      if (name === 'dismiss') continue; // Esc is per-component chrome, not a shell branch
      expect(glyphs.has(glyph as string), `handler implements ${name} (${glyph}) with no palette label`).toBe(true);
    }
  });
});
