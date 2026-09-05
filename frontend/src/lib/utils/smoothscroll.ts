const WHEEL_STEP = 120;
const LINE_HEIGHT = 40;
const PAGE_RATIO = 0.9;
const TAU = 62;
const EPSILON = 0.5;
const DRIFT = 2;
const GESTURE = 400;

export type WheelSample = {
  deltaY: number;
  deltaMode: number;
  wheelDeltaY?: number;
};

export type ScrollMetrics = {
  scrollTop: number;
  scrollHeight: number;
  clientHeight: number;
};

export function wheelPixels(sample: WheelSample, viewport: number): number {
  if (sample.deltaMode === 1) return sample.deltaY * LINE_HEIGHT;
  if (sample.deltaMode === 2) return sample.deltaY * viewport * PAGE_RATIO;
  return sample.deltaY;
}

// Chromium and WebKit both report a notched wheel as whole 120-unit ticks,
// while touchpads report fine-grained deltas the OS already smooths.
export function coarseWheel(sample: WheelSample): boolean {
  if (sample.deltaMode !== 0) return true;
  const ticks = Math.abs(sample.wheelDeltaY ?? 0);
  if (ticks === 0) return Number.isInteger(sample.deltaY) && Math.abs(sample.deltaY) >= LINE_HEIGHT;
  return ticks >= WHEEL_STEP && ticks % WHEEL_STEP === 0;
}

export function canScroll(metrics: ScrollMetrics, direction: number): boolean {
  const max = metrics.scrollHeight - metrics.clientHeight;
  if (max <= 1) return false;
  return direction < 0 ? metrics.scrollTop > 1 : metrics.scrollTop < max - 1;
}

export function settle(current: number, target: number, elapsed: number): number {
  if (elapsed <= 0) return current;
  const next = current + (target - current) * (1 - Math.exp(-elapsed / TAU));
  return Math.abs(target - next) < EPSILON ? target : next;
}

function clamp(value: number, max: number): number {
  return Math.min(Math.max(value, 0), max);
}

type Ride = {
  node: HTMLElement;
  target: number;
  pos: number;
  applied: number;
  last: number;
};

export function installSmoothScroll(): () => void {
  const reduced = window.matchMedia('(prefers-reduced-motion: reduce)');
  let ride: Ride | null = null;
  let raf = 0;
  let cachedFrom: Element | null = null;
  let cachedNode: HTMLElement | null = null;
  let cachedAt = 0;

  const scrolls = (node: HTMLElement) => {
    if (node.scrollHeight - node.clientHeight <= 1) return false;
    const overflow = getComputedStyle(node).overflowY;
    return overflow === 'auto' || overflow === 'scroll';
  };

  const pick = (from: EventTarget | null, direction: number, now: number) => {
    const start = from instanceof Element ? from : null;
    if (!start) return null;
    if (start === cachedFrom && cachedNode?.isConnected && now - cachedAt < GESTURE && canScroll(cachedNode, direction)) {
      cachedAt = now;
      return cachedNode;
    }
    let node: Element | null = start;
    while (node) {
      if (node instanceof HTMLElement && scrolls(node) && canScroll(node, direction)) {
        cachedFrom = start;
        cachedNode = node;
        cachedAt = now;
        return node;
      }
      node = node.parentElement;
    }
    return null;
  };

  const frame = (now: number) => {
    raf = 0;
    if (!ride) return;
    const node = ride.node;
    if (!node.isConnected || Math.abs(node.scrollTop - ride.applied) > DRIFT) {
      ride = null;
      return;
    }
    const max = Math.max(0, node.scrollHeight - node.clientHeight);
    ride.target = clamp(ride.target, max);
    // scrollTop reads back rounded, so integrating from it stalls short of the
    // target: the float position lives here instead.
    const next = settle(clamp(ride.pos, max), ride.target, now - ride.last);
    ride.last = now;
    ride.pos = next;
    node.scrollTop = next;
    ride.applied = node.scrollTop;
    if (next === ride.target) {
      ride = null;
      return;
    }
    raf = requestAnimationFrame(frame);
  };

  const onWheel = (event: WheelEvent) => {
    if (event.defaultPrevented || event.ctrlKey) return;
    if (reduced.matches || document.documentElement.classList.contains('no-anim')) return;
    if (event.deltaY === 0 || Math.abs(event.deltaX) > Math.abs(event.deltaY)) return;
    const sample: WheelSample = {
      deltaY: event.deltaY,
      deltaMode: event.deltaMode,
      wheelDeltaY: (event as WheelEvent & { wheelDeltaY?: number }).wheelDeltaY,
    };
    if (!coarseWheel(sample)) {
      ride = null;
      return;
    }
    const now = performance.now();
    const node = pick(event.target, Math.sign(event.deltaY), now);
    if (!node) {
      ride = null;
      return;
    }
    event.preventDefault();
    if (!ride || ride.node !== node || Math.abs(node.scrollTop - ride.applied) > DRIFT) {
      ride = { node, target: node.scrollTop, pos: node.scrollTop, applied: node.scrollTop, last: now };
    }
    const max = Math.max(0, node.scrollHeight - node.clientHeight);
    ride.target = clamp(ride.target + wheelPixels(sample, node.clientHeight), max);
    if (!raf) raf = requestAnimationFrame(frame);
  };

  window.addEventListener('wheel', onWheel, { passive: false });

  return () => {
    window.removeEventListener('wheel', onWheel);
    if (raf) cancelAnimationFrame(raf);
    raf = 0;
    ride = null;
  };
}
