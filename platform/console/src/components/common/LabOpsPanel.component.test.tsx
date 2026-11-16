import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import { LabOpsPanel } from './LabOpsPanel';
import { getOrCreateSigner } from '@/lib/custody';
import type { Lab } from '@/types';

// Migrated from react-test-renderer (LabOpsPanel.test.tsx). The old suite
// reached into the tree for `props.onChange` / `props.onConfirm` / `props
// .requireText`, which never proved the control an operator sees is the
// control that fires. These assertions drive the real inputs and buttons in a
// Chromium page; the envelope, the typed gate and the refusal slot are read
// back from the request bodies and the rendered DOM.
//
// No `vi.mock('@/lib/api')`: the real tenant wrappers run and the POSTs are
// answered at the fetch boundary, so the body the console actually sends (the
// golden params_hash included) is what is asserted.

const TENANT_WRITE = {
  id: 'lab-a',
  display_name: 'lab',
  autonomy_level: 'L1',
  status: 'active',
  aborted_jobs_count: 0,
};

type Call = { url: string; method: string; path: string; body: Record<string, unknown> };

function lab(partial: Partial<Lab> & { id: string }): Lab {
  return {
    display_name: 'lab',
    autonomy_level: 'L1',
    status: 'active',
    ...partial,
  };
}

function ok(payload: unknown = TENANT_WRITE) {
  return { ok: true, status: 200, json: async () => payload };
}

/** Records every console write; answers at the fetch boundary. */
function stubWrites(answer: (call: Call) => unknown = () => ok()) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const parsed = new URL(String(url));
    const call: Call = {
      url: String(url),
      method: (init?.method ?? 'GET').toUpperCase(),
      path: parsed.pathname,
      body: init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : {},
    };
    calls.push(call);
    const routed = answer(call);
    if (routed !== undefined) return routed;
    return ok();
  });
  return calls;
}

function disabled(locator: { element(): Element }): boolean {
  return (locator.element() as HTMLButtonElement).disabled;
}

function textOf(screen: { container: HTMLElement }): string {
  return screen.container.textContent ?? '';
}

async function renderPanel(adminPubkey: string | undefined, onChanged: () => void = () => undefined) {
  return render(<LabOpsPanel lab={lab({ id: 'lab-a', admin_pubkey_hex: adminPubkey })} onChanged={onChanged} />);
}

/** Opens the typed refund gate (amount + reason filled, Refund clicked). */
async function openRefundGate(
  adminPubkey: string,
  opts: { amount?: string; reason?: string; onChanged?: () => void } = {},
) {
  const screen = await renderPanel(adminPubkey, opts.onChanged);
  await expect.poll(() => textOf(screen)).toContain('this browser key');
  await screen.getByLabelText('Refund amount for lab-a').fill(opts.amount ?? '10');
  await screen.getByLabelText('Refund reason for lab-a').fill(opts.reason ?? 'overcharge correction');
  await screen.getByRole('button', { name: 'Refund' }).click();
  const dialog = screen.getByRole('alertdialog');
  await expect.element(dialog).toBeInTheDocument();
  return { screen, dialog };
}

let PUB = '';
beforeAll(async () => {
  PUB = (await getOrCreateSigner()).publicKeyHex;
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('LabOpsPanel (U1-b) in a real browser', () => {
  it('ceilings carry the golden params_hash end-to-end (UI → envelope → Go)', async () => {
    const calls = stubWrites();
    const screen = await renderPanel(PUB);
    await expect.poll(() => textOf(screen)).toContain('this browser key');

    await screen.getByLabelText('New ceiling for lab-a').fill('150');
    await screen.getByRole('button', { name: 'Set ceiling' }).click();

    // Wave-3: ceiling stages a ConfirmDialog (reversible, no typed text).
    const dialog = screen.getByRole('alertdialog');
    await expect.element(dialog).toBeInTheDocument();
    expect(dialog.element().getAttribute('aria-label')).toBe('Set lab-a ceiling to $150.00?');
    const confirm = dialog.getByRole('button', { name: 'Set ceiling' });
    await expect.poll(() => disabled(confirm), { timeout: 3000 }).toBe(false);
    await confirm.click();

    await expect.poll(() => calls.length).toBe(1);
    expect(calls[0].url).toBe('http://localhost:8080/api/tenants/ceiling');
    expect(calls[0].method).toBe('POST');
    const body = calls[0].body;
    expect(body.tenant_id).toBe('lab-a');
    expect(body.ceiling_usd).toBe(150);
    const action = body.action as Record<string, string>;
    expect(action.action).toBe('set_ceiling');
    // Golden: the exact Go ParamsHashOf for {"ceiling_usd":150}.
    expect(action.params_hash).toBe('95169ffc05abd4ab865019f68df37924f29d682856c4c4e9b929b74069e0e891');
    expect(action.pubkey_hex).toBe(PUB);
    expect(action.signature_hex).toMatch(/^[0-9a-f]{128}$/);
    // Success closes the gate.
    expect(screen.container.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('mismatched keys fall back to the lab-admin session without an envelope', async () => {
    const calls = stubWrites();
    const screen = await renderPanel('ab'.repeat(32));
    await expect.poll(() => textOf(screen)).toContain('is not this lab');

    await screen.getByLabelText('New ceiling for lab-a').fill('150');
    await screen.getByRole('button', { name: 'Set ceiling' }).click();

    const dialog = screen.getByRole('alertdialog');
    const confirm = dialog.getByRole('button', { name: 'Set ceiling' });
    await expect.poll(() => disabled(confirm), { timeout: 3000 }).toBe(false);
    await confirm.click();

    await expect.poll(() => calls.length).toBe(1);
    expect(calls[0].path).toBe('/api/tenants/ceiling');
    expect(calls[0].body.ceiling_usd).toBe(150);
    expect('action' in calls[0].body).toBe(false);
  });

  it('bootstraps an admin-less lab with no envelope (first write wins)', async () => {
    const calls = stubWrites(() => ok({ id: 'lab-new' }));
    const screen = await render(<LabOpsPanel lab={lab({ id: 'lab-new' })} onChanged={() => undefined} />);
    await expect.poll(() => textOf(screen)).toContain('No admin registered');

    await screen.getByLabelText('Operator token').fill('test-token');
    const register = screen.getByRole('button', { name: /Register this browser key/ });
    await expect.poll(() => disabled(register)).toBe(false);
    await register.click();

    await expect.poll(() => calls.length).toBe(1);
    expect(calls[0].url).toBe('http://localhost:8080/api/tenants/admin-key');
    expect(calls[0].body.tenant_id).toBe('lab-new');
    // The browser key is minted by custody, not invented by the UI.
    expect(String(calls[0].body.new_admin_pubkey_hex)).toMatch(/^[0-9a-f]{64}$/);
    expect('action' in calls[0].body).toBe(false);
  });

  it('refund without a reason refuses client-side (no network)', async () => {
    const calls = stubWrites();
    const screen = await renderPanel(PUB);

    await screen.getByLabelText('Refund amount for lab-a').fill('10');
    await screen.getByRole('button', { name: 'Refund' }).click();

    expect(calls).toHaveLength(0);
    await expect.poll(() => screen.getByRole('alert').element().textContent).toContain('needs a reason');
    expect(screen.container.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('suspend without confirm sends nothing', async () => {
    const calls = stubWrites();
    const screen = await renderPanel(PUB);

    await screen.getByRole('button', { name: 'Suspend lab lab-a' }).click();

    // Wave-3 replaced the native confirm() with a trapped dialog: the click
    // stages it and nothing is sent until the operator confirms.
    const dialog = screen.getByRole('alertdialog');
    await expect.element(dialog).toBeInTheDocument();
    expect(dialog.element().getAttribute('aria-label')).toBe('Suspend lab lab-a?');
    expect(calls).toHaveLength(0);

    await dialog.getByRole('button', { name: 'Cancel' }).click();
    expect(screen.container.querySelector('[role="alertdialog"]')).toBeNull();
    expect(calls).toHaveLength(0);
  });
});

describe('LabOpsPanel refund typed gate (Wave-2) in a real browser', () => {
  it('requires the exact "$X.XX": wrong text keeps confirm disabled, exact text enables + fires', async () => {
    const calls = stubWrites();
    let changed = 0;
    const screen = await renderPanel(PUB, () => {
      changed += 1;
    });
    await expect.poll(() => textOf(screen)).toContain('this browser key');
    await screen.getByLabelText('Refund amount for lab-a').fill('10');
    await screen.getByLabelText('Refund reason for lab-a').fill('overcharge correction');
    await screen.getByRole('button', { name: 'Refund' }).click();

    // No native confirm, no network until the typed acknowledgement.
    expect(calls).toHaveLength(0);
    const dialog = screen.getByRole('alertdialog');
    await expect.element(dialog).toBeInTheDocument();
    expect(dialog.element().getAttribute('aria-label')).toBe('Refund $10.00 to lab-a?');
    expect(textOf(screen)).toContain('Audited, irreversible');

    const confirm = dialog.getByRole('button', { name: 'Refund $10.00' });
    const typed = dialog.getByRole('textbox');
    expect(disabled(confirm)).toBe(true);

    // Wrong text: never enables, however long the dialog has been open.
    await typed.fill('10.00');
    expect(disabled(confirm)).toBe(true);
    expect(calls).toHaveLength(0);

    // Exact text is necessary but not sufficient: the settle window is part
    // of the guard, so the control enables only once it has elapsed. (The
    // window itself is pinned in ConfirmDialog.component.test.tsx.)
    await typed.fill('$10.00');
    await expect.poll(() => disabled(confirm), { timeout: 3000 }).toBe(false);
    await confirm.click();

    await expect.poll(() => calls.length).toBe(1);
    expect(calls[0].url).toBe('http://localhost:8080/api/tenants/refund');
    expect(calls[0].body.amount_usd).toBe(10);
    expect(calls[0].body.reason).toBe('overcharge correction');
    // The parent refetch hook fires once, after the accepted write.
    await expect.poll(() => changed).toBe(1);
    // Success closes the gate.
    await expect.poll(() => screen.container.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('busy disables the gate; backend refusal surfaces in the dialog error slot with no silent close', async () => {
    const calls: Call[] = [];
    let release: (() => void) | null = null;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
      calls.push({
        url: String(url),
        method: (init?.method ?? 'GET').toUpperCase(),
        path: new URL(String(url)).pathname,
        body: init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : {},
      });
      await held;
      return { ok: false, status: 403, text: async () => 'not-admin', json: async () => ({ error: 'not-admin' }) };
    });

    const { screen, dialog } = await openRefundGate(PUB);
    await dialog.getByRole('textbox').fill('$10.00');
    const confirm = dialog.getByRole('button', { name: 'Refund $10.00' });
    await expect.poll(() => disabled(confirm), { timeout: 3000 }).toBe(false);

    // Element handle: the label changes to "Working…" while the write is in
    // flight, so the locator would stop resolving.
    const confirmEl = dialog.element().querySelectorAll('button')[1] as HTMLButtonElement;
    await userEvent.click(confirmEl);

    await expect.poll(() => confirmEl.textContent).toContain('Working');
    expect(confirmEl.disabled).toBe(true);

    release!();
    // Refused: the gate stays open with the backend reason verbatim.
    await expect.poll(() => textOf(screen), { timeout: 3000 }).toContain('not-admin');
    expect(screen.container.querySelectorAll('[role="alertdialog"]')).toHaveLength(1);
    expect(calls).toHaveLength(1);
    expect(calls[0].path).toBe('/api/tenants/refund');
  });

  it('backdrop/cancel dismisses the gate with no network', async () => {
    const calls = stubWrites();
    const { screen } = await openRefundGate(PUB);

    // Backdrop click funnels to onCancel. Dispatch on the backdrop element
    // itself: the panel is centred, so a hit-tested click would land inside
    // it (where the dialog stops propagation).
    const backdrop = screen.container.querySelector('[role="presentation"]') as HTMLElement;
    expect(backdrop).not.toBeNull();
    backdrop.click();
    await expect.poll(() => screen.container.querySelector('[role="alertdialog"]')).toBeNull();
    expect(calls).toHaveLength(0);
    await screen.unmount();

    // Escape funnels to the same onCancel (trapped dialog contract).
    const second = await openRefundGate(PUB);
    expect(second.screen.container.querySelectorAll('[role="alertdialog"]')).toHaveLength(1);
    await userEvent.keyboard('{Escape}');
    await expect.poll(() => second.screen.container.querySelector('[role="alertdialog"]')).toBeNull();
    expect(calls).toHaveLength(0);
  });
});
