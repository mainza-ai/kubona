import { useEffect, useState } from 'react';

/** Clicks that spend, abort, or release the air-gap wait this long. */
export const CONSEQUENTIAL_SETTLE_MS = 500;

/**
 * A consequential control stays inert until the settle window passes, so a
 * double-click or a focus-and-Enter on open cannot decide it.
 *
 * The window re-arms every time the control becomes active. State is
 * adjusted during render (the documented "derive from a changed prop"
 * pattern) and the adjusted value is used in that same render, so the
 * control never paints one frame as clickable.
 */
export function useConsequentialSettle(active: boolean, settleMs = CONSEQUENTIAL_SETTLE_MS): boolean {
  const inert = active && settleMs > 0;
  const [released, setReleased] = useState(!inert);
  const [wasActive, setWasActive] = useState(active);

  let effectiveReleased = released;
  if (wasActive !== active) {
    setWasActive(active);
    setReleased(!inert);
    effectiveReleased = !inert;
  }

  useEffect(() => {
    if (!inert || released) return;
    const timer = setTimeout(() => setReleased(true), settleMs);
    return () => clearTimeout(timer);
  }, [inert, released, settleMs]);

  return !inert || effectiveReleased;
}
