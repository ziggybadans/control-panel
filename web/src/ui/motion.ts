// View Transitions helper: layout changes (dashboard reorder/resize/hide,
// tab underline) morph smoothly in browsers that support the API and apply
// instantly everywhere else — and for anyone preferring reduced motion.

import { flushSync } from "react-dom";

type DocumentWithVT = Document & {
  startViewTransition?: (cb: () => void) => unknown;
};

export function withViewTransition(update: () => void) {
  const doc = document as DocumentWithVT;
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  if (!doc.startViewTransition || reduced) {
    update();
    return;
  }
  doc.startViewTransition(() => {
    // The API snapshots before/after frames; React must commit synchronously
    // inside the callback for the "after" snapshot to be correct.
    flushSync(update);
  });
}
