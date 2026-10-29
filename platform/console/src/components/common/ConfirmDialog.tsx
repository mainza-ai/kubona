import { useEffect, useRef, useState } from 'react';
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
  error?: string | null;
  onConfirm: () => void | Promise<void>;
  onCancel: () => void;
};

/**
 * Typed confirmation dialog for irreversible or money-moving actions.
 * Replaces native confirm() (invisible to screen readers, no typed
 * acknowledgement). Trapped + returns focus; Escape cancels. The confirm
 * button stays disabled until the typed text matches exactly.
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
  onConfirm,
  onCancel,
}: Omit<ConfirmDialogProps, 'open'>) {
  const panelRef = useRef<HTMLDivElement>(null);
  const [typed, setTyped] = useState('');
  useFocusTrap(true, panelRef);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onCancel();
      }
    };
    window.addEventListener('keydown', onKey, true);
    return () => window.removeEventListener('keydown', onKey, true);
  }, [onCancel]);

  const canConfirm = !busy && (requireText == null || typed === requireText);

  return (
    <div className="fixed inset-0 z-[90] flex items-center justify-center bg-black/70 p-4" role="presentation" onClick={onCancel}>
      <div
        ref={panelRef}
        role="alertdialog"
        aria-modal="true"
        aria-label={title}
        aria-describedby="confirm-dialog-body"
        className="w-[min(28rem,calc(100vw-3rem))] rounded-xl border border-white/15 bg-[#0b0e1a] p-5 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-base font-semibold text-white">{title}</h2>
        <div id="confirm-dialog-body" className="mt-2 text-sm text-slate-300">
          {body}
        </div>
        {requireText != null && (
          <label className="mt-3 block text-xs text-slate-400">
            Type <code className="rounded bg-white/10 px-1 font-mono">{requireText}</code> to confirm
            <input
              type="text"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              disabled={busy}
              autoComplete="off"
              className="mt-1 block w-full rounded-md border border-white/15 bg-white/5 px-2 py-1.5 text-sm text-white placeholder:text-slate-500"
            />
          </label>
        )}
        {error != null && (
          <p role="alert" className="mt-2 text-sm text-red-300">
            {error}
          </p>
        )}
        <div className="mt-4 flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            disabled={busy}
            className="rounded-md border border-white/15 px-3 py-1.5 text-sm text-slate-200 hover:bg-white/5 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void onConfirm()}
            disabled={!canConfirm}
            className={`rounded-md px-3 py-1.5 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-40 ${
              danger ? 'bg-red-600 text-white hover:bg-red-500' : 'bg-cyan-400 text-black hover:bg-cyan-300'
            }`}
          >
            {busy ? 'Working…' : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
