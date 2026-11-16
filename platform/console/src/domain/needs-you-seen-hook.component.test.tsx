import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import {
  NEEDS_YOU_SEEN_KEY,
  resetNeedsYouSeenForTests,
  useNeedsYouSeen,
} from './needs-you-seen';

function Probe({ mark, prune }: { mark: string[]; prune: string[] | null }) {
  const { seen, markSeen, prune: pruneSeen } = useNeedsYouSeen();
  return (
    <div>
      <button type="button" aria-label="mark" onClick={() => markSeen(mark)} />
      <button type="button" aria-label="prune" onClick={() => pruneSeen(prune)} />
      <span data-testid="size">{seen.size}</span>
      <span data-testid="list">{[...seen].sort().join(',')}</span>
    </div>
  );
}

beforeEach(() => {
  window.sessionStorage.removeItem(NEEDS_YOU_SEEN_KEY);
  resetNeedsYouSeenForTests();
});

describe('useNeedsYouSeen in a real browser', () => {
  it('marks items seen, prunes inactive signatures, and re-raises a returning row', async () => {
    const screen = await render(<Probe mark={['ob_1@t1', 'ob_2@t1']} prune={['ob_2@t1']} />);
    const size = screen.getByTestId('size');
    const list = screen.getByTestId('list');
    await expect.element(size).toHaveTextContent('0');

    await screen.getByRole('button', { name: 'mark' }).click();
    await expect.element(list).toHaveTextContent('ob_1@t1,ob_2@t1');

    // A decision removes ob_1; its signature leaves the store.
    await screen.getByRole('button', { name: 'prune' }).click();
    await expect.element(list).toHaveTextContent('ob_2@t1');
    expect(list.element().textContent).toBe('ob_2@t1');
    await screen.unmount();
  });

  it('treats a new update stamp as a distinct unseen item', async () => {
    const screen = await render(<Probe mark={['ob_1@t2']} prune={null} />);
    await screen.getByRole('button', { name: 'mark' }).click();
    await expect.element(screen.getByTestId('list')).toHaveTextContent('ob_1@t2');
    await screen.unmount();
  });

  it('does not prune while the list is still loading', async () => {
    const screen = await render(<Probe mark={['ob_3@t1']} prune={null} />);
    await screen.getByRole('button', { name: 'mark' }).click();
    const size = screen.getByTestId('size');
    await expect.element(size).toHaveTextContent('1');
    await screen.getByRole('button', { name: 'prune' }).click();
    await expect.element(size).toHaveTextContent('1');
    expect(size.element().textContent).toBe('1');
    await screen.unmount();
  });
});
