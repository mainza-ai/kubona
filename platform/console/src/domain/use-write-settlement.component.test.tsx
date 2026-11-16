import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { useWriteSettlement } from './use-write-settlement';

function Probe() {
  const { write, begin, applied } = useWriteSettlement();
  const id = React.useRef<string | null>(null);
  return (
    <button type="button" onClick={() => {
      if (!id.current) id.current = begin('doing work');
      else applied(id.current, 'done');
    }}>
      {write.phase}:{write.detail}
    </button>
  );
}

describe('useWriteSettlement in a real browser', () => {
  it('does not report accepted as applied', async () => {
    const screen = await render(<Probe />);
    const button = screen.getByRole('button');
    await button.click();
    expect(button.element().textContent).toContain('accepted');
    expect(button.element().textContent).toContain('doing work');
    await button.click();
    expect(button.element().textContent).toContain('applied');
    expect(button.element().textContent).toContain('done');
  });
});
