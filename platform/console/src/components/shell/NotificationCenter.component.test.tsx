import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { NotificationCenter } from './NotificationCenter';

const rows = [
  {
    key: 'event:1', kind: 'job', severity: 'info', source: 'job_settled',
    message: 'Job d2fae5e8 completed', action: 'job:d2fae5e8',
    created_at: '2026-11-15T21:33:49Z', read_at: '', resolved_at: '',
  },
  {
    key: 'obligation:ob-1', kind: 'obligation', severity: 'warning', source: 'claim',
    message: 'Decision needed: claim clm-1', action: 'obligation:ob-1',
    created_at: '2026-11-15T21:30:00Z', read_at: '', resolved_at: '',
  },
];

describe('NotificationCenter in a real browser', () => {
  it('lists notifications, marks them read, and resolves one', async () => {
    const onMarkRead = vi.fn();
    const onResolve = vi.fn();
    const screen = await render(
      <NotificationCenter
        open
        notifications={rows}
        unread={2}
        onClose={vi.fn()}
        onMarkRead={onMarkRead}
        onResolve={onResolve}
      />,
    );
    await expect.element(screen.getByText('Job d2fae5e8 completed')).toBeVisible();
    await expect.element(screen.getByText('Decision needed: claim clm-1')).toBeVisible();
    await expect.element(screen.getByText(/2 unread/)).toBeVisible();
    expect(onMarkRead).toHaveBeenCalledWith(['event:1', 'obligation:ob-1']);
    await screen.getByRole('button', { name: 'Resolve notification obligation:ob-1' }).click();
    expect(onResolve).toHaveBeenCalledWith('obligation:ob-1');
  });

  it('shows an honest empty state', async () => {
    const screen = await render(
      <NotificationCenter
        open
        notifications={[]}
        unread={0}
        onClose={vi.fn()}
        onMarkRead={vi.fn()}
        onResolve={vi.fn()}
      />,
    );
    await expect.element(screen.getByText(/No notifications yet/)).toBeVisible();
  });
});
