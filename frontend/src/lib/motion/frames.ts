import { documentMotion } from './motion';

/**
 * A motion that is a function of time: `draw(now)` on every animation frame, `now` in ms since the
 * epoch, so two windows (or two processes) that agree on when the motion began draw the same frame.
 * It runs until `draw` returns false or the returned stop is called. With motion off (motion.ts) or
 * on a host without animation frames it draws nothing and answers null: the caller shows its still
 * frame instead.
 */
export function runFrames(draw: (now: number) => boolean | void): (() => void) | null {
  if (!documentMotion() || typeof requestAnimationFrame !== 'function') return null;
  let frame = 0;
  let running = true;
  const tick = () => {
    if (!running) return;
    if (draw(Date.now()) === false) {
      running = false;
      return;
    }
    frame = requestAnimationFrame(tick);
  };
  frame = requestAnimationFrame(tick);
  return () => {
    running = false;
    cancelAnimationFrame(frame);
  };
}
