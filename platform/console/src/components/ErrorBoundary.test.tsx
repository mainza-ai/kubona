import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { ErrorBoundary } from './ErrorBoundary';
import * as api from '@/lib/api';

afterEach(() => {
  vi.restoreAllMocks();
});

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

function Thrower(): React.ReactNode {
  throw new Error('boom from fixture');
}

describe('ErrorBoundary', () => {
  it('renders children when nothing throws', async () => {
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(
        <ErrorBoundary>
          <span>healthy child</span>
        </ErrorBoundary>,
      );
    });
    expect(textOf(root!.root)).toContain('healthy child');
  });

  it('renders the fail-closed fallback with retry when a child throws, and logs via reportError', async () => {
    const spy = vi.spyOn(api, 'reportError').mockImplementation(() => undefined);
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(
        <ErrorBoundary>
          <Thrower />
        </ErrorBoundary>,
      );
    });
    const text = textOf(root!.root);
    expect(text).toContain('Something broke rendering this view');
    const retry = root!.root.findAllByType('button').find((b) => textOf(b).includes('Retry'));
    expect(retry).toBeDefined();
    expect(root!.root.findByProps({ role: 'alert' })).toBeDefined();
    expect(spy).toHaveBeenCalled();
    expect(text).not.toContain('boom from fixture');
    consoleSpy.mockRestore();
  });
});
