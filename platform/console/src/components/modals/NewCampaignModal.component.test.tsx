import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { NewCampaignModal } from './NewCampaignModal';

type CreateParams = {
  title: string;
  targetMolecule: string;
  spendLimitUSD: number;
  fresh: boolean;
};

type Props = React.ComponentProps<typeof NewCampaignModal>;

function renderModal(props: Partial<Props> = {}) {
  return render(
    <NewCampaignModal
      isOpen
      onClose={() => undefined}
      onCreate={async () => undefined}
      {...props}
    />,
  );
}

async function submit(screen: Awaited<ReturnType<typeof renderModal>>) {
  await screen.getByRole('button', { name: 'Create Research Campaign' }).click();
}

describe('NewCampaignModal inline validation in a real browser (Wave-2)', () => {
  it('exposes the title and custom-molecule fields by label', async () => {
    const screen = await renderModal();

    const title = screen.getByLabelText('Research campaign name or hypothesis').element();
    const molecule = screen.getByLabelText('Custom chemical formula or SMILES').element();
    expect(title.tagName).toBe('INPUT');
    expect(molecule.tagName).toBe('INPUT');

    // Every visible label points at an element that exists in the document.
    for (const label of Array.from(screen.container.querySelectorAll('label'))) {
      if (label.className === 'sr-only') continue;
      const htmlFor = label.getAttribute('for');
      if (htmlFor) {
        expect(document.getElementById(htmlFor), `label ${htmlFor}`).not.toBeNull();
      }
    }
  });

  it('offers no pre-computed reference demo', async () => {
    const screen = await renderModal();
    const text = screen.container.textContent ?? '';

    expect(text).not.toContain('Reference Demo');
    expect(text).not.toContain('Synthesize Reference Baseline');
  });

  it('rejects a short title inline and never calls onCreate', async () => {
    const onCreate = vi.fn<(p: CreateParams) => Promise<void>>();
    const screen = await renderModal({ onCreate });

    await screen.getByLabelText('Research campaign name or hypothesis').fill('ab');
    await submit(screen);

    const titleError = screen.getByTestId('campaign-title-error');
    await expect.element(titleError).toBeInTheDocument();
    expect(titleError.element().textContent).toMatch(/at least 3/);
    expect(onCreate).not.toHaveBeenCalled();
    await expect.element(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('rejects an empty title inline', async () => {
    const onCreate = vi.fn<(p: CreateParams) => Promise<void>>();
    const screen = await renderModal({ onCreate });

    await screen.getByLabelText('Research campaign name or hypothesis').fill('   ');
    await submit(screen);

    await expect.element(screen.getByTestId('campaign-title-error')).toBeInTheDocument();
    expect(onCreate).not.toHaveBeenCalled();
  });

  it('rejects an invalid custom molecule inline without falling back to the preset', async () => {
    const onCreate = vi.fn<(p: CreateParams) => Promise<void>>();
    const screen = await renderModal({ onCreate });
    const molecule = screen.getByLabelText('Custom chemical formula or SMILES');

    await molecule.fill('H2 O!!');
    await submit(screen);

    await expect.element(screen.getByTestId('campaign-molecule-error')).toBeInTheDocument();
    expect(onCreate).not.toHaveBeenCalled();
    // Still open, invalid value preserved (no silent preset substitution).
    expect((molecule.element() as HTMLInputElement).value).toBe('H2 O!!');
    await expect.element(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('creates with the valid custom molecule (trimmed) and closes', async () => {
    const onCreate = vi.fn<(p: CreateParams) => Promise<void>>().mockResolvedValue(undefined);
    const onClose = vi.fn(() => {});
    const screen = await renderModal({ onCreate, onClose });

    await screen.getByLabelText('Custom chemical formula or SMILES').fill('  CH4  ');
    await submit(screen);

    await expect.poll(() => onCreate.mock.calls.length).toBe(1);
    expect(onCreate.mock.calls[0][0]).toEqual({
      title: 'CH4 Ground State Optimization',
      targetMolecule: 'CH4',
      spendLimitUSD: 100,
      fresh: true,
    });
    await expect.poll(() => onClose.mock.calls.length).toBe(1);
  });

  it('keeps the modal OPEN and shows the server refusal verbatim on failure', async () => {
    const onCreate = vi.fn<(p: CreateParams) => Promise<void>>().mockRejectedValue(
      new Error('quota exhausted for tenant lab-a (verbatim)'),
    );
    const onClose = vi.fn(() => {});
    const screen = await renderModal({ onCreate, onClose });

    await submit(screen);

    await expect.element(screen.getByTestId('campaign-create-error')).toBeInTheDocument();
    expect(screen.container.textContent).toContain('quota exhausted for tenant lab-a (verbatim)');
    expect(onClose).not.toHaveBeenCalled();
    await expect.element(screen.getByRole('dialog')).toBeInTheDocument();
  });
});
