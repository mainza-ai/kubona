import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { useTranscript } from './transcript-store';
import type { ChatMessage } from '@/types';

function Probe({ id }: { id: string }) {
  const { messages, setMessages } = useTranscript('s1');
  return (
    <button type="button" onClick={() => setMessages((previous) => [...previous, { id, session_id: 's1', role: 'user', content: id, created_at: '' } as ChatMessage])}>
      {messages.length}
    </button>
  );
}

describe('transcript store in a real browser', () => {
  it('shares messages between simultaneous consumers', async () => {
    const screen = await render(<><Probe id="a" /><Probe id="b" /></>);
    const buttons = screen.getByRole('button').elements() as HTMLElement[];
    await buttons[0].click();
    await expect.poll(() => buttons.map((button) => button.textContent)).toEqual(['1', '1']);
  });
});
