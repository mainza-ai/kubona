import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { AttentionProvider, useAttention } from './attention-context';

// Migrated from react-test-renderer (attention-context.test.tsx). The old
// suite captured the context value from an effect and called the API
// directly; here every action is a real click on a real button, so the
// provider's state machine is exercised through the same path surfaces use.

function Probe() {
  const a = useAttention();
  return (
    <div>
      <span data-testid="alert">{a.alert ? `${a.alert.channel}:${a.alert.level}` : 'none'}</span>
      <span data-testid="alert-id">{a.alert?.id ?? 0}</span>
      <span data-testid="duplicates">{a.alert?.duplicates ?? 0}</span>
      <span data-testid="presence">{a.presence}</span>
      <span data-testid="muted">{a.muted ? 'muted' : 'audible'}</span>
      <span data-testid="tightened">{a.tightened.join(',') || 'none'}</span>
      <button type="button" onClick={() => a.notify({ level: 'anomalous', source: 'vault' })}>
        notify-anomalous-vault
      </button>
      <button type="button" onClick={() => a.notify({ level: 'notable', source: 'vault' })}>
        notify-notable-vault
      </button>
      <button type="button" onClick={() => a.notify({ level: 'notable', source: 'lab' })}>
        notify-notable-lab
      </button>
      <button type="button" onClick={() => a.clear()}>
        clear
      </button>
      <button type="button" onClick={() => a.resolve(true)}>
        resolve-acted
      </button>
      <button type="button" onClick={() => a.resolve(false)}>
        resolve-dismissed
      </button>
      <button type="button" onClick={() => a.setPresence('eyes')}>
        presence-eyes
      </button>
      <button type="button" onClick={() => a.setPresence('ear')}>
        presence-ear
      </button>
      <button type="button" onClick={() => a.setPresence('away')}>
        presence-away
      </button>
      <button type="button" onClick={() => a.setPresence('hands')}>
        presence-hands
      </button>
      <button type="button" onClick={() => a.toggleMute()}>
        toggle-mute
      </button>
    </div>
  );
}

function withProvider() {
  return (
    <AttentionProvider>
      <Probe />
    </AttentionProvider>
  );
}

// The mute preference is a module singleton backed by localStorage; the
// provider only reads it, but a leak between files would still be real state.
afterEach(() => {
  window.localStorage.removeItem('kubona-ambient-muted');
});

describe('attention context in a real browser', () => {
  it('notifies anomalous and suppresses notable when away', async () => {
    const screen = await render(withProvider());

    await screen.getByRole('button', { name: 'notify-anomalous-vault' }).click();
    await expect.element(screen.getByTestId('alert')).toHaveTextContent('summon:anomalous');

    await screen.getByRole('button', { name: 'presence-away' }).click();
    await expect.element(screen.getByTestId('presence')).toHaveTextContent('away');

    await screen.getByRole('button', { name: 'clear' }).click();
    await expect.element(screen.getByTestId('alert')).toHaveTextContent('none');

    await screen.getByRole('button', { name: 'notify-notable-vault' }).click();
    // Away drops non-summons: no alert stored.
    expect(screen.getByTestId('alert').element().textContent).toBe('none');

    // Summons still land while away — suppression is level-scoped, not global.
    await screen.getByRole('button', { name: 'notify-anomalous-vault' }).click();
    await expect.element(screen.getByTestId('alert')).toHaveTextContent('summon:anomalous');
    await screen.unmount();
  });

  it('stub is safe without a provider', async () => {
    const screen = await render(<Probe />);

    expect(screen.getByTestId('alert').element().textContent).toBe('none');
    expect(screen.getByTestId('presence').element().textContent).toBe('hands');
    expect(screen.getByTestId('muted').element().textContent).toBe('muted');

    // Must not throw, and must not change any state.
    await screen.getByRole('button', { name: 'notify-anomalous-vault' }).click();
    await screen.getByRole('button', { name: 'toggle-mute' }).click();
    expect(screen.getByTestId('alert').element().textContent).toBe('none');
    expect(screen.getByTestId('muted').element().textContent).toBe('muted');
    expect(screen.getByTestId('tightened').element().textContent).toBe('none');
    await screen.unmount();
  });

  it('collapses duplicate alerts instead of stacking (R-11)', async () => {
    const screen = await render(withProvider());

    // 'eyes' lets notable through (hands would suppress it pre-collapse).
    await screen.getByRole('button', { name: 'presence-eyes' }).click();
    await expect.element(screen.getByTestId('presence')).toHaveTextContent('eyes');

    await screen.getByRole('button', { name: 'notify-notable-vault' }).click();
    await expect.element(screen.getByTestId('alert')).toHaveTextContent('glance:notable');
    const firstId = screen.getByTestId('alert-id').element().textContent;
    expect(Number(firstId)).toBeGreaterThan(0);
    expect(screen.getByTestId('duplicates').element().textContent).toBe('0');

    await screen.getByRole('button', { name: 'notify-notable-vault' }).click();
    // Same id (no new alert), duplicate count incremented.
    await expect.element(screen.getByTestId('duplicates')).toHaveTextContent('1');
    expect(screen.getByTestId('alert-id').element().textContent).toBe(firstId);

    await screen.getByRole('button', { name: 'notify-notable-vault' }).click();
    await expect.element(screen.getByTestId('duplicates')).toHaveTextContent('2');
    expect(screen.getByTestId('alert-id').element().textContent).toBe(firstId);

    // Different source does not collapse.
    await screen.getByRole('button', { name: 'notify-notable-lab' }).click();
    await expect.element(screen.getByTestId('duplicates')).toHaveTextContent('0');
    expect(screen.getByTestId('alert-id').element().textContent).not.toBe(firstId);
    await screen.unmount();
  });

  it('switches presence through the dial values', async () => {
    const screen = await render(withProvider());

    await expect.element(screen.getByTestId('presence')).toHaveTextContent('hands');
    for (const p of ['eyes', 'ear', 'away', 'hands'] as const) {
      await screen.getByRole('button', { name: `presence-${p}` }).click();
      await expect.element(screen.getByTestId('presence')).toHaveTextContent(p);
    }
    await screen.unmount();
  });

  it('starts muted by default and persists explicit unmute/mute toggles', async () => {
    const screen = await render(withProvider());

    await expect.element(screen.getByTestId('muted')).toHaveTextContent('muted');
    await screen.getByRole('button', { name: 'toggle-mute' }).click();
    await expect.element(screen.getByTestId('muted')).toHaveTextContent('audible');
    expect(window.localStorage.getItem('kubona-ambient-muted')).toBe('0');

    await screen.getByRole('button', { name: 'toggle-mute' }).click();
    await expect.element(screen.getByTestId('muted')).toHaveTextContent('muted');
    expect(window.localStorage.getItem('kubona-ambient-muted')).toBe('1');
    await screen.unmount();
  });

  it('tracks precision and tightens low-precision sources (R-11)', async () => {
    const screen = await render(withProvider());
    expect(screen.getByTestId('tightened').element().textContent).toBe('none');

    // 1 acted + 4 dismissed = 5 rated, precision 0.2 -> tightened.
    // Use anomalous so alerts survive default 'hands' presence.
    await screen.getByRole('button', { name: 'notify-anomalous-vault' }).click();
    await expect.element(screen.getByTestId('alert')).toHaveTextContent('summon:anomalous');
    await screen.getByRole('button', { name: 'resolve-acted' }).click();
    await expect.element(screen.getByTestId('alert')).toHaveTextContent('none');

    for (let i = 0; i < 4; i++) {
      await screen.getByRole('button', { name: 'notify-anomalous-vault' }).click();
      await expect.element(screen.getByTestId('alert')).toHaveTextContent('summon:anomalous');
      await screen.getByRole('button', { name: 'resolve-dismissed' }).click();
      await expect.element(screen.getByTestId('alert')).toHaveTextContent('none');
    }
    await expect.element(screen.getByTestId('tightened')).toHaveTextContent('vault');

    // A tightened source's glance-level alert escalates one band instead of
    // being delivered at glance (the whole point of the precision tracker).
    await screen.getByRole('button', { name: 'presence-eyes' }).click();
    await expect.element(screen.getByTestId('presence')).toHaveTextContent('eyes');
    await screen.getByRole('button', { name: 'notify-notable-vault' }).click();
    await expect.element(screen.getByTestId('alert')).toHaveTextContent('nudge:notable');
    await screen.unmount();
  });
});
