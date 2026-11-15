'use client';

import React from 'react';
import { reportError } from '@/lib/api';

interface ErrorBoundaryProps {
  children: React.ReactNode;
}

interface ErrorBoundaryState {
  hasError: boolean;
}

// Fail-closed render guard: a throwing child renders the fallback, never a
// half-painted view. All logging funnels through reportError (the future
// Sentry hook) — no ad-hoc console calls here.
export class ErrorBoundary extends React.Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { hasError: false };

  static getDerivedStateFromError(): ErrorBoundaryState {
    return { hasError: true };
  }

  componentDidCatch(error: unknown, info: React.ErrorInfo): void {
    reportError('ErrorBoundary', error instanceof Error ? error : new Error(String(error)));
    if (info?.componentStack) {
      reportError('ErrorBoundary:componentStack', info.componentStack);
    }
  }

  private handleRetry = (): void => {
    // Fail-closed recovery: a full reload re-boots clean state. Guarded so
    // non-browser runtimes (tests) reset local state instead of crashing.
    if (typeof window !== 'undefined' && typeof window.location?.reload === 'function') {
      window.location.reload();
      return;
    }
    this.setState({ hasError: false });
  };

  render(): React.ReactNode {
    if (this.state.hasError) {
      return (
        <div role="alert" className="p-6 rounded-card bg-danger-wash border border-danger-edge text-danger-ink space-y-3">
          <p className="text-body-strong font-semibold">Something broke rendering this view — nothing loaded, nothing assumed.</p>
          <button
            type="button"
            onClick={this.handleRetry}
            className="px-4 py-1.5 rounded-row bg-fill hover:bg-fill-hover text-text text-label font-semibold border border-border"
          >
            Retry
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}

export default ErrorBoundary;
