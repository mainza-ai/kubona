'use client';

import React, { useEffect, useId, useRef } from 'react';

export function Menu({
  label,
  children,
  className = '',
}: {
  label: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <details className={`k-menu ${className}`.trim()}>
      <summary className="k-menu-trigger">{label}</summary>
      <div className="k-menu-panel" role="menu">{children}</div>
    </details>
  );
}

export function Dialog({
  open,
  onClose,
  title,
  children,
  className = '',
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
  className?: string;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;
    if (open && !dialog.open && typeof dialog.showModal === 'function') {
      dialog.showModal();
    }
    if (!open && dialog.open && typeof dialog.close === 'function') {
      dialog.close();
    }
  }, [open]);

  useEffect(() => {
    const dialog = ref.current;
    if (!dialog || typeof HTMLDialogElement === 'undefined') return;
    if ('closedBy' in HTMLDialogElement.prototype) return;
    const outside = (e: PointerEvent) => {
      const r = dialog.getBoundingClientRect();
      return e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom;
    };
    let pressedOutside = false;
    const onDown = (e: PointerEvent) => {
      pressedOutside = outside(e);
    };
    const onUp = (e: PointerEvent) => {
      if (pressedOutside && outside(e)) {
        dialog.close();
      }
    };
    dialog.addEventListener('pointerdown', onDown);
    dialog.addEventListener('pointerup', onUp);
    return () => {
      dialog.removeEventListener('pointerdown', onDown);
      dialog.removeEventListener('pointerup', onUp);
    };
  }, []);

  return (
    <dialog
      ref={ref}
      {...({ closedby: 'any' } as Record<string, string>)}
      className={`k-dialog ${className}`.trim()}
      aria-labelledby={titleId}
      onClose={() => {
        if (ref.current) ref.current.returnValue = '';
        onClose();
      }}
      onCancel={(event) => { event.preventDefault(); onClose(); }}
    >
      <div className="k-dialog-head">
        <h2 id={titleId}>{title}</h2>
        <button type="button" className="k-dialog-close" onClick={onClose} aria-label="Close dialog">Close</button>
      </div>
      <div className="k-dialog-body">{children}</div>
    </dialog>
  );
}

export function Tooltip({
  label,
  children,
  className = '',
}: {
  label: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <span className={`k-tooltip ${className}`.trim()} data-tooltip={label}>
      {children}
      <span className="k-tooltip-content" role="tooltip">{label}</span>
    </span>
  );
}
