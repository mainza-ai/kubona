'use client';

import React from 'react';
import { isChunkLoadError } from './lazy-retry';

type Props = {
  children: React.ReactNode;
  /** Named in the refusal, e.g. "figure". */
  label?: string;
};

type State = {
  error: Error | null;
  attempt: number;
};

/**
 * D5: a lazy chart that fails to load must say so and offer a recovery. A
 * stale chunk (deployment replaced the hashed file) reloads the document; an
 * ordinary render failure remounts the boundary once per retry.
 */
export class LazyBoundary extends React.Component<Props, State> {
  state: State = { error: null, attempt: 0 };

  static getDerivedStateFromError(error: Error): Partial<State> {
    return { error };
  }

  private retry = () => {
    if (this.state.error && isChunkLoadError(this.state.error)) {
      if (typeof window !== 'undefined') {
        window.location.reload();
        return;
      }
    }
    this.setState((state) => ({ error: null, attempt: state.attempt + 1 }));
  };

  render() {
    const label = this.props.label ?? 'figure';
    if (this.state.error) {
      const chunk = isChunkLoadError(this.state.error);
      return (
        <div
          role="alert"
          className="rounded-control border border-danger-edge bg-danger-wash px-3 py-2 text-danger-ink"
        >
          <p className="text-label font-semibold">
            {chunk ? `${label} could not be loaded` : `${label} could not be rendered`}
          </p>
          <p className="mt-0.5 text-body">
            {chunk
              ? 'The page changed while this view was loading. Reload to fetch the current version.'
              : 'The data is unchanged; retry the view.'}
          </p>
          <button
            type="button"
            onClick={this.retry}
            className="mt-2 rounded-control-compact border border-current px-2 py-0.5 font-code text-label hover:bg-danger-edge/20"
          >
            {chunk ? 'Reload' : 'Retry'}
          </button>
        </div>
      );
    }
    return <React.Fragment key={this.state.attempt}>{this.props.children}</React.Fragment>;
  }
}
