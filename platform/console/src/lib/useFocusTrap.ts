import { useEffect, type RefObject } from 'react';

// Keyboard trap for custom modal overlays: Tab cycles inside the panel,
// focus lands on the first control when the overlay opens and returns to
// the invoker when it closes. Native <dialog> would give this for free;
// these overlays predate it, so the trap is explicit and tested.
export function useFocusTrap(active: boolean, panelRef: RefObject<HTMLElement | null>) {
  useEffect(() => {
    if (!active || typeof document === 'undefined') return;
    const panel = panelRef.current;
    if (!panel) return;
    const prev = document.activeElement as HTMLElement | null;
    const selectables = () =>
      Array.from(
        panel.querySelectorAll<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
        )
      ).filter((el) => !el.hasAttribute('disabled') && el.getAttribute('aria-hidden') !== 'true');
    selectables()[0]?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Tab') return;
      const items = selectables();
      if (items.length === 0) return;
      const first = items[0];
      const last = items[items.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };
    panel.addEventListener('keydown', onKey);
    return () => {
      panel.removeEventListener('keydown', onKey);
      if (prev && typeof prev.focus === 'function') prev.focus();
    };
  }, [active, panelRef]);
}
