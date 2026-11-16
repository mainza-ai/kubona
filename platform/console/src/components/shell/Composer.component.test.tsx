import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import { Composer } from './Composer';

// The dock hydrates the active model label on mount and again when the model
// morph opens, so both endpoints are stubbed by URL.
function stubLLM() {
  vi.stubGlobal('fetch', async (url: string) => {
    if (String(url).includes('/api/llm/models')) {
      return { ok: true, status: 200, json: async () => ({ models: [{ id: 'model-a' }] }) };
    }
    return {
      ok: true,
      status: 200,
      json: async () => ({
        active_provider: 'omlx',
        active_model: 'model-a',
        role_assignments: { copilot: { role: 'copilot', provider: 'omlx', model: 'model-a' } },
      }),
    };
  });
}

afterEach(() => vi.unstubAllGlobals());

describe('Composer in a real browser', () => {
  it('sends on Enter and clears the field', async () => {
    stubLLM();
    const onSubmit = vi.fn(async () => {});
    const screen = await render(<Composer onSubmit={onSubmit} contextLabel="Campaign: H2" />);

    await expect.element(screen.getByText('Campaign: H2')).toBeVisible();
    const area = screen.getByLabelText('Ask Kubona');
    await area.fill('run the campaign');
    await userEvent.keyboard('{Enter}');

    await expect.poll(() => onSubmit.mock.calls.length).toBe(1);
    expect(onSubmit).toHaveBeenCalledWith('run the campaign');
    await expect.poll(() => (area.element() as HTMLTextAreaElement).value).toBe('');
  });

  it('attaches a readable text file, detaches it, and only opens settings on request', async () => {
    stubLLM();
    const onAttach = vi.fn();
    const onDetach = vi.fn();
    const onOpenModel = vi.fn();
    const screen = await render(
      <Composer onSubmit={vi.fn()} onAttach={onAttach} onDetach={onDetach} onOpenModel={onOpenModel} />,
    );

    const fileInput = screen.container.querySelector('input[type="file"]');
    expect(fileInput).not.toBeNull();
    await userEvent.upload(fileInput!, new File(['hello'], 'notes.txt', { type: 'text/plain' }));

    await expect.poll(() => onAttach.mock.calls.length).toBe(1);
    expect(onAttach).toHaveBeenCalledWith('notes.txt', 'hello');
    await expect.element(screen.getByText('notes.txt')).toBeVisible();

    await screen.getByRole('button', { name: 'Remove notes.txt' }).click();
    await expect.poll(() => onDetach.mock.calls.length).toBe(1);
    expect(onDetach).toHaveBeenCalledWith('notes.txt');
    await expect.poll(() => screen.container.textContent).not.toContain('notes.txt');

    // Opening the model hub is not the same as leaving for LLM Settings.
    await screen.getByRole('button', { name: 'Open Model Hub' }).click();
    expect(onOpenModel).not.toHaveBeenCalled();
    const settings = screen.getByRole('button', { name: /Open LLM Settings/ });
    await expect.element(settings).toBeVisible();
    await settings.click();
    await expect.poll(() => onOpenModel.mock.calls.length).toBe(1);
  });

  it('keeps Shift+Enter and IME composition as text entry', async () => {
    stubLLM();
    const onSubmit = vi.fn();
    const screen = await render(<Composer onSubmit={onSubmit} />);
    const area = screen.getByLabelText('Ask Kubona');

    await area.fill('line');
    await userEvent.keyboard('{Shift>}{Enter}{/Shift}');
    expect((area.element() as HTMLTextAreaElement).value).toContain('line');

    // Chromium cannot be driven into a real IME composition, so the composing
    // Enter is dispatched as a native KeyboardEvent with isComposing set.
    area.element().dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true, isComposing: true }),
    );

    expect(onSubmit).not.toHaveBeenCalled();
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(onSubmit).not.toHaveBeenCalled();
  });
});
