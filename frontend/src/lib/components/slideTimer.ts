/**
 * One run of a carousel slide's timer: `done` after `ms`, and `frame` with the time run so far on
 * every animation frame (for the progress drawn on the slide's control). The returned stop ends the
 * run early and says how long it ran, so a held carousel can run out only what is left later.
 */
export function runSlide(ms: number, done: () => void, frame: (ran: number) => void): () => number {
  const start = Date.now();
  const timer = setTimeout(done, Math.max(0, ms));
  let id = 0;
  const draw = () => {
    frame(Date.now() - start);
    id = requestAnimationFrame(draw);
  };
  if (typeof requestAnimationFrame === 'function') id = requestAnimationFrame(draw);
  return () => {
    clearTimeout(timer);
    if (typeof cancelAnimationFrame === 'function') cancelAnimationFrame(id);
    return Date.now() - start;
  };
}

/** Back above this share of the carousel in view, its timer runs again. */
export const RUN_IN_VIEW = 0.4;
/** Below this share in view the timer holds; between the two nothing changes, so a slow scroll does not flap. */
export const HOLD_IN_VIEW = 0.25;

/**
 * Tells whether the carousel is in view of the scroller it sits in (the window when none), with
 * hysteresis between HOLD_IN_VIEW and RUN_IN_VIEW. `change` is called only when the answer flips; it
 * starts as in view and stays so where the browser has no IntersectionObserver.
 */
export function watchInView(node: HTMLElement, change: (inView: boolean) => void): () => void {
  if (typeof IntersectionObserver === 'undefined') return () => {};
  let root = node.parentElement;
  while (root && !/auto|scroll/.test(getComputedStyle(root).overflowY)) root = root.parentElement;
  let inView = true;
  const observer = new IntersectionObserver(
    (entries) => {
      const ratio = entries.at(-1)?.intersectionRatio;
      if (ratio === undefined) return;
      const next = ratio >= RUN_IN_VIEW ? true : ratio < HOLD_IN_VIEW ? false : inView;
      if (next === inView) return;
      inView = next;
      change(next);
    },
    { root, threshold: [0, HOLD_IN_VIEW, RUN_IN_VIEW, 1] },
  );
  observer.observe(node);
  return () => observer.disconnect();
}
