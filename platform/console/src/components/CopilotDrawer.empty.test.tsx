import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { CopilotDrawer } from './CopilotDrawer';
import * as api from '@/lib/api';

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>();
  return {
    ...actual,
    fetchMessages: vi.fn(async () => []),
    sendChatMessage: vi.fn(),
    streamChatMessage: vi.fn(),
    createSession: vi.fn(),
    wasRecentAuthFailure: vi.fn(() => false),
  };
});

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

describe('CopilotDrawer no-campaign state', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.fetchMessages).mockResolvedValue([]);
  });

  it('does not invent an H2 campaign or swarm context', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<CopilotDrawer sessionId="s" />);
    });
    const text = textOf(tree!.root);
    expect(text).toContain('No campaign selected. Create or open a campaign to start.');
    expect(text).not.toContain('Autonomous Swarm Initialized');
    expect(text).not.toContain('Hydrogen Molecule (H₂)');
  });

  it('offers a create-campaign action when no campaign exists', async () => {
    const onCreateCampaign = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<CopilotDrawer sessionId="s" onCreateCampaign={onCreateCampaign} />);
    });
    const button = tree!.root.findAllByType('button').find((b) => textOf(b).includes('Create campaign'));
    expect(button).toBeDefined();
    act(() => { button!.props.onClick(); });
    expect(onCreateCampaign).toHaveBeenCalledTimes(1);
  });
});
