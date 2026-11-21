'use client';

import { useEffect, type RefObject } from 'react';

export interface DismissiblePopoverOptions {
  /** CSS selector for trigger buttons that toggle this popover (avoids close+reopen bounce). */
  ignoreSelector?: string;
}

/**
 * Dismisses a floating popover/tray when the user clicks outside of it or
 * presses Escape. Ignores interactions inside an open modal alertdialog so
 * nested confirmation dialogs (e.g. Delete campaign) stay intact.
 */
export function useDismissiblePopover(
  open: boolean,
  panelRef: RefObject<HTMLElement | null>,
  onClose: () => void,
  options?: DismissiblePopoverOptions,
): void {
  const ignoreSelector = options?.ignoreSelector;

  useEffect(() => {
    if (!open || typeof document === 'undefined') return;

    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as HTMLElement | null;
      if (!target) return;
      if (panelRef.current && panelRef.current.contains(target)) return;
      if (ignoreSelector && target.closest(ignoreSelector)) return;
      if (target.closest('[role="alertdialog"]')) return;
      onClose();
    };

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      if (document.querySelector('[role="alertdialog"]')) return;
      event.stopPropagation();
      onClose();
    };

    document.addEventListener('pointerdown', onPointerDown);
    window.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('pointerdown', onPointerDown);
      window.removeEventListener('keydown', onKeyDown);
    };
  }, [open, panelRef, onClose, ignoreSelector]);
}
