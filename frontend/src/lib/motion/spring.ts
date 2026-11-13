import { documentMotion } from './motion';

/**
 * A number driven by a physical spring, for a shape that changes size while the user may be looking
 * at it (the live island's width and height). Not svelte/motion's `Spring`: its stiffness and damping
 * are per-frame factors, while a preset here is a spring as designers name one (`response`, the
 * seconds of one undamped swing, and `damping`, 1 for no overshoot), the same feel the prototype's
 * island has. Retargeted mid-flight it keeps its velocity, so a change of mind never jerks.
 *
 * With motion off (`documentMotion()`: the animations setting or a reduced-motion request, read at
 * every `set`) it jumps to its target and asks for no frame. Its owner calls `stop()` when it goes.
 */

export interface SpringPreset {
  response: number;
  damping: number;
}

export const SPRINGS = {
  smooth: { response: 0.32, damping: 1 },
  bouncy: { response: 0.4, damping: 0.7 },
} as const satisfies Record<string, SpringPreset>;

export type SpringName = keyof typeof SPRINGS;

const REST = 0.001;
/** The longest step a frame may take: a background tab or a long task must not fling the value. */
const MAX_STEP_S = 0.064;

function coefficients(p: SpringPreset) {
  return { k: ((2 * Math.PI) / p.response) ** 2, c: (4 * Math.PI * p.damping) / p.response };
}

/** Semi-implicit Euler, sub-stepped at 240 Hz for stability. Mass is 1. */
function step(x: number, v: number, target: number, dt: number, k: number, c: number): [number, number] {
  const n = Math.max(1, Math.ceil(dt * 240));
  const h = dt / n;
  for (let i = 0; i < n; i++) {
    v += (-k * (x - target) - c * v) * h;
    x += v * h;
  }
  return [x, v];
}

export class SpringValue {
  private x: number;
  private v = 0;
  private target: number;
  private raf = 0;
  private last = 0;
  private readonly k: number;
  private readonly c: number;

  constructor(
    initial: number,
    private readonly onUpdate: (value: number) => void,
    preset: SpringName | SpringPreset = 'smooth',
    private readonly motion: () => boolean = documentMotion,
  ) {
    ({ k: this.k, c: this.c } = coefficients(typeof preset === 'string' ? SPRINGS[preset] : preset));
    this.x = initial;
    this.target = initial;
  }

  get value(): number {
    return this.x;
  }

  get velocity(): number {
    return this.v;
  }

  set(target: number, opts: { instant?: boolean } = {}): void {
    this.target = target;
    if (opts.instant || !this.motion() || typeof requestAnimationFrame !== 'function') {
      this.stop();
      this.x = target;
      this.v = 0;
      this.onUpdate(target);
      return;
    }
    if (this.raf) return;
    this.last = performance.now();
    this.raf = requestAnimationFrame(this.frame);
  }

  stop(): void {
    if (this.raf) cancelAnimationFrame(this.raf);
    this.raf = 0;
  }

  private frame = (now: number) => {
    if (!this.raf) return;
    const dt = Math.min(MAX_STEP_S, Math.max(0, (now - this.last) / 1000));
    this.last = now;
    [this.x, this.v] = step(this.x, this.v, this.target, dt, this.k, this.c);
    const span = Math.max(1, Math.abs(this.target));
    if (Math.abs(this.target - this.x) < REST * span && Math.abs(this.v) < REST * 10 * span) {
      this.x = this.target;
      this.v = 0;
      this.raf = 0;
      this.onUpdate(this.x);
      return;
    }
    this.onUpdate(this.x);
    this.raf = requestAnimationFrame(this.frame);
  };
}
