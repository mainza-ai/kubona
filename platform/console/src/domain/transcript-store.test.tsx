import { describe, expect, it } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
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

describe('transcript store', () => {
  it('shares messages between simultaneous consumers', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => { tree = TestRenderer.create(<><Probe id="a" /><Probe id="b" /></>); });
    const buttons = tree!.root.findAllByType('button');
    act(() => { buttons[0].props.onClick(); });
    expect(tree!.root.findAllByType('button').map((button) => button.props.children)).toEqual([1, 1]);
  });
});
