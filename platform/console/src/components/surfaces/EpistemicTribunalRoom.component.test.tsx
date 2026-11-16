import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import { EpistemicTribunalRoom } from './EpistemicTribunalRoom';
import { CONSEQUENTIAL_SETTLE_MS } from '@/lib/consequential';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function turn(role: string, name: string) {
  return { role, name, content: `${role} statement`, verdict: 'pass', turn_index: 1, timestamp: '' };
}

const TURNS = [
  turn('ansatz-scout', 'Scout'),
  turn('disconfirmation-officer', 'Officer'),
  turn('epistemic-scribe', 'Scribe'),
  turn('device-auditor', 'Auditor'),
];

type Call = { method: string; path: string; body: unknown };

function json(body: unknown) {
  return { ok: true, status: 200, json: async () => body };
}

function refusal(status: number, message: string) {
  const body = JSON.stringify({ error: message });
  return { ok: false, status, text: async () => body, json: async () => ({ error: message }) };
}

function campaignBody(sessionId: string, targetSystem: string) {
  return {
    session_id: sessionId,
    target_system: targetSystem,
    quantum_factors: {},
    classical_anchor: {},
    trajectory: [],
    updated_at: '',
  };
}

type TribunalStub = {
  /** Body for POST /api/sessions/{id}/campaign/synthesize. */
  synthesize?: () => unknown;
  /** Body for POST /api/ledger/append. */
  ledger?: () => unknown;
};

// The chamber reads the real transport wrappers, so every route it touches is
// answered at fetch level. The mount-time message load answers empty so the
// chamber starts idle; deliberation is only ever the explicit Rerun.
function installFetch(stub: TribunalStub = {}) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    const method = (init?.method ?? 'GET').toUpperCase();
    const body = init?.body === undefined ? undefined : JSON.parse(String(init.body));
    calls.push({ method, path, body });
    if (method === 'GET' && /\/messages$/.test(path)) {
      return json({ messages: [] });
    }
    if (path === '/api/chat/tribunal') {
      return json({ tribunal: { turns: TURNS }, assistant_message: {} });
    }
    if (path.endsWith('/campaign/synthesize')) {
      return stub.synthesize
        ? stub.synthesize()
        : json(campaignBody(path.split('/')[3], 'H2'));
    }
    if (path === '/api/ledger/append') {
      return stub.ledger ? stub.ledger() : json({ ok: true, record_hash: 'rh-1', event_id: 'e1' });
    }
    throw new Error(`unexpected fetch ${method} ${path}`);
  });
  return calls;
}

function rerun(screen: Awaited<ReturnType<typeof render>>) {
  return screen.getByRole('button', { name: /Rerun Tribunal/ });
}

function synthesize(screen: Awaited<ReturnType<typeof render>>) {
  return screen.getByRole('button', { name: /Synthesize Campaign/ });
}

function commit(screen: Awaited<ReturnType<typeof render>>) {
  return screen.getByRole('button', { name: /Commit Discovery to Merkle Ledger/ });
}

function disabled(locator: { element(): Element }): boolean {
  return (locator.element() as HTMLButtonElement).disabled;
}

/** Reaches supermajority consensus through the real deliberation call. */
async function reachConsensus(sessionId = 'sess-12345678') {
  const screen = await render(<EpistemicTribunalRoom sessionId={sessionId} targetSystem="H2" />);
  expect(disabled(commit(screen))).toBe(true);
  await rerun(screen).click();
  await expect.poll(() => disabled(commit(screen)), { timeout: 3000 }).toBe(false);
  return screen;
}

describe('EpistemicTribunalRoom synthesize (U2-b) in a real browser', () => {
  it('synthesize is disabled before consensus and posts the topic after', async () => {
    const calls = installFetch();
    let synthesized: { target_system?: string } | null = null;

    const screen = await render(
      <EpistemicTribunalRoom
        sessionId="s1"
        targetSystem="H2"
        onCampaignSynthesized={(c) => { synthesized = c as unknown as { target_system?: string }; }}
      />
    );
    expect(disabled(synthesize(screen))).toBe(true);

    await rerun(screen).click();
    await expect.poll(() => disabled(synthesize(screen)), { timeout: 3000 }).toBe(false);

    await synthesize(screen).click();
    await expect.poll(() => screen.container.textContent, { timeout: 3000 })
      .toContain('ledger-sealed server-side');

    const posted = calls.filter((c) => c.path.endsWith('/campaign/synthesize'));
    expect(posted).toHaveLength(1);
    expect(posted[0].method).toBe('POST');
    expect(posted[0].path).toBe('/api/sessions/s1/campaign/synthesize');
    expect((posted[0].body as { prompt: string }).prompt).toContain('H2');
    expect(synthesized).toMatchObject({ target_system: 'H2' });
  });

  it('refusal leaves the campaign untouched with a reason', async () => {
    installFetch({ synthesize: () => refusal(500, 'llm unavailable') });
    let called = false;

    const screen = await render(
      <EpistemicTribunalRoom sessionId="s1" onCampaignSynthesized={() => { called = true; }} />
    );
    await rerun(screen).click();
    await expect.poll(() => disabled(synthesize(screen)), { timeout: 3000 }).toBe(false);
    await synthesize(screen).click();

    await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain('campaign unchanged');
    expect(called).toBe(false);
  });
});

describe('EpistemicTribunalRoom ledger-commit typed gate (Wave-2) in a real browser', () => {
  it('stages a typed short-id gate: wrong text disabled, exact text enables + commits', async () => {
    const calls = installFetch();
    const screen = await reachConsensus('sess-12345678');

    // Single-click no more: the button stages the dialog, commits nothing.
    await commit(screen).click();
    const gate = screen.getByRole('alertdialog');
    await expect.element(gate).toBeVisible();
    expect(calls.filter((c) => c.path === '/api/ledger/append')).toHaveLength(0);
    // The gate names the exact short-id it expects.
    expect(gate.element().textContent).toContain('sess-123');

    const typed = gate.element().querySelector('input') as HTMLInputElement;
    const confirm = screen.getByRole('button', { name: /^Commit to ledger$/ });

    await userEvent.fill(typed, 'sess-12');
    expect(disabled(confirm)).toBe(true);

    await userEvent.fill(typed, 'sess-123');
    // Exact id is necessary but not sufficient: the settle window keeps a
    // focus-and-Enter on open from committing the ledger entry.
    expect(disabled(confirm)).toBe(true);
    await expect.poll(() => disabled(confirm), { timeout: CONSEQUENTIAL_SETTLE_MS + 3000 }).toBe(false);

    await confirm.click();
    await expect.poll(
      () => calls.filter((c) => c.path === '/api/ledger/append').length,
      { timeout: 3000 },
    ).toBe(1);
    const append = calls.find((c) => c.path === '/api/ledger/append')!;
    expect((append.body as { event_type: string }).event_type).toBe('tribunal_commit');
    await expect.poll(() => screen.container.textContent, { timeout: 3000 })
      .toContain('Discovery Committed to Merkle Ledger');
    expect(screen.container.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('ledger refusal stays open in the dialog error slot (no silent close)', async () => {
    installFetch({ ledger: () => refusal(403, 'ledger refused: quorum stale') });
    const screen = await reachConsensus();

    await commit(screen).click();
    const gate = screen.getByRole('alertdialog');
    const typed = gate.element().querySelector('input') as HTMLInputElement;
    const confirm = screen.getByRole('button', { name: /^Commit to ledger$/ });

    await userEvent.fill(typed, 'sess-123');
    await expect.poll(() => disabled(confirm), { timeout: CONSEQUENTIAL_SETTLE_MS + 3000 }).toBe(false);
    await confirm.click();

    await expect.element(screen.getByRole('alert')).toHaveTextContent('ledger refused: quorum stale');
    // Still open — nothing sealed.
    expect(screen.container.querySelector('[role="alertdialog"]')).not.toBeNull();
    expect(screen.container.textContent).not.toContain('Discovery Committed to Merkle Ledger');
  });

  it('backdrop/cancel dismisses without committing', async () => {
    const calls = installFetch();
    const screen = await reachConsensus();

    await commit(screen).click();
    const gate = screen.getByRole('alertdialog');
    await expect.element(gate).toBeVisible();
    // Corner of the fixed backdrop: the panel stops propagation at its centre,
    // so the backdrop's own dismiss handler is exercised directly.
    const backdrop = screen.container.querySelector('[role="presentation"]') as HTMLElement;
    expect(backdrop).not.toBeNull();
    backdrop.click();
    await expect.poll(() => screen.container.querySelector('[role="alertdialog"]'), { timeout: 3000 }).toBeNull();

    // Escape funnels to the same onCancel.
    await commit(screen).click();
    await expect.element(screen.getByRole('alertdialog')).toBeVisible();
    await userEvent.keyboard('{Escape}');
    await expect.poll(() => screen.container.querySelector('[role="alertdialog"]'), { timeout: 3000 }).toBeNull();

    expect(calls.filter((c) => c.path === '/api/ledger/append')).toHaveLength(0);
    expect(screen.container.textContent).not.toContain('Discovery Committed to Merkle Ledger');
  });

  it('quorum display carries progressbar semantics with an accessible name', async () => {
    installFetch();
    const screen = await render(<EpistemicTribunalRoom sessionId="s1" targetSystem="H2" />);
    const bar = screen.getByRole('progressbar', { name: 'Tribunal quorum consensus' });
    // Presence + ARIA state only: the browser page carries no app stylesheet,
    // so a zero-content track has no layout box to be "visible".
    expect(bar.element().getAttribute('role')).toBe('progressbar');
    expect(bar.element().getAttribute('aria-valuemin')).toBe('0');
    expect(bar.element().getAttribute('aria-valuemax')).toBe('100');
    expect(bar.element().getAttribute('aria-valuenow')).toBe('0');

    await rerun(screen).click();
    // Four corroborated turns → 100%.
    await expect.poll(() => bar.element().getAttribute('aria-valuenow'), { timeout: 3000 }).toBe('100');
    expect(screen.container.textContent).toContain('100.0%');
  });
});
