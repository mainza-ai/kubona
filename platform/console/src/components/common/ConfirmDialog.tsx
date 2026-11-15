import { useEffect, useRef, useState } from 'react';
import { CONSEQUENTIAL_SETTLE_MS, useConsequentialSettle } from '@/lib/consequential';
import { useFocusTrap } from '@/lib/useFocusTrap';

type ConfirmDialogProps = {
  open: boolean;
  title: string;
  body: React.ReactNode;
  /** Exact text the user must type to enable confirm. If omitted, a single explicit confirm click suffices (for reversible actions only). */
  requireText?: string;
  confirmLabel: string;
  danger?: boolean;
  busy?: boolean;
  /** Inert window before confirm can be chosen. 0 only for tests of other gates. */
  settleMs?: number;
  error?: string | null;
  onConfirm: () => void | Promise<void>;
  onCancel: () => void;
};

/**
 * Typed confirmation dialog for irreversible or money-moving actions.
 * Replaces native confirm() (invisible to screen readers, no typed
 * acknowledgement). Trapped + returns focus. Escape closes without
 * confirming. Confirm stays inert for the settle window, and focus lands
 * on Cancel unless the operator must type.
 */
export default function ConfirmDialog(props: ConfirmDialogProps) {
  if (!props.open) return null;
  // Remount per open so the typed acknowledgement always starts empty —
  // no in-effect setState needed.
  return <DialogBody key="open" {...props} />;
}

function DialogBody({
  title,
  body,
  requireText,
  confirmLabel,
  danger,
  busy,
  error,
  settleMs = CONSEQUENTIAL_SETTLE_MS,
  onConfirm,
  onCancel,
}: Omit<ConfirmDialogProps, 'open'>) {
  const panelRef = useRef<HTMLDivElement>(null);
  const [typed, setTyped] = useState('');
  const settled = useConsequentialSettle(true, settleMs);
  useFocusTrap(true, panelRef);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        onCancel();
      }
    };
    window.addEventListener('keydown', onKey, true);
    return () => window.removeEventListener('keydown', onKey, true);
  }, [onCancel]);

  const canConfirm = settled && !busy && (requireText == null || typed === requireText);

  return (
    <div className="fixed inset-0 z-[90] flex items-center justify-center bg-black/70 p-4" role="presentation" onClick={onCancel}>
      <div
        ref={panelRef}
        role="alertdialog"
        aria-modal="true"
        aria-label={title}
        aria-describedby="confirm-dialog-body"
        className="w-[min(28rem,calc(100vw-3rem))] rounded-row border border-border-strong bg-float p-5 shadow-[var(--shadow-float)]"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-body-large font-semibold text-text">{title}</h2>
        <div id="confirm-dialog-body" className="mt-2 text-body-strong text-label-secondary">
          {body}
        </div>
        {requireText != null && (
          <label className="mt-3 block text-label text-muted">
            Type <code className="rounded-chip bg-fill px-1 font-mono">{requireText}</code> to confirm
            <input
              type="text"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              disabled={busy}
              autoFocus
              autoComplete="off"
              className="mt-1 block w-full rounded-inset border border-border bg-field px-2 py-1.5 text-body-strong text-text placeholder:text-faint"
            />
          </label>
        )}
        {error != null && (
          <p role="alert" className="mt-2 text-body-strong text-danger-ink">
            {error}
          </p>
        )}
        <div className="mt-4 flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            autoFocus={requireText == null}
            disabled={busy}
            className="rounded-inset border border-border px-3 py-1.5 text-body-strong text-label-secondary hover:bg-fill-hover disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void onConfirm()}
            data-settle={settled ? 'ready' : 'pending'}
            disabled={!canConfirm}
            className={`rounded-inset px-3 py-1.5 text-body-strong font-semibold disabled:cursor-not-allowed disabled:opacity-40 ${
              danger ? 'bg-rose-600 text-on-accent hover:bg-rose-500' : 'bg-accent text-on-accent hover:bg-lit'
            }`}
          >
            {busy ? 'Working…' : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
