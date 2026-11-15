'use client';

import React from 'react';

/**
 * C3 — one bad item costs that item, never the canvas. A boundary wraps each
 * card and each chart; the fallback keeps the record visible and offers a
 * retry without touching the record or the rest of the scene.
 */
export class RenderBoundary extends React.Component<
  {
    label: string;
    fallback: (reset: () => void) => React.ReactNode;
    children: React.ReactNode;
  },
  { failed: boolean }
> {
  state: { failed: boolean } = { failed: false };

  static getDerivedStateFromError(): { failed: boolean } {
    return { failed: true };
  }

  componentDidCatch(error: unknown): void {
    // Log once with the card identity; the UI stays calm and local.
    console.error('[workbench] card render failed:', this.props.label, error);
  }

  reset = (): void => {
    this.setState({ failed: false });
  };

  render(): React.ReactNode {
    return this.state.failed ? this.props.fallback(this.reset) : this.props.children;
  }
}
