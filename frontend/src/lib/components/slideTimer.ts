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
