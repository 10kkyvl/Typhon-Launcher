import { mixPixels, type BackdropLook, type Pixels } from './pixels';

export interface PainterDeps {
  /** The baked picture for an art URL in a look; null when the art cannot be loaded or read. */
  picture(url: string, look: BackdropLook): Promise<Pixels | null>;
  /** Draws the screen buffer. */
  put(): void;
  frame(step: (now: number) => void): number;
  cancelFrame(id: number): void;
  now(): number;
}

/** Baked pictures kept for going back and forth between games; each is a few kilobytes. */
const CACHE_SIZE = 32;

/**
 * Draws the backdrop into one small buffer: a picture per art and look, baked once, and a cross-fade
 * between the picture on screen and the next one. Only the newest target is ever drawn, a picture that
 * fails stays failed for that look, and frames are requested only while a fade runs.
 */
export class BackdropPainter {
  private readonly pictures = new Map<string, Promise<Pixels | null>>();
  private readonly from: Uint8ClampedArray;
  private readonly empty: Uint8ClampedArray;
  private drawn = '';
  private generation = 0;
  private frameId = 0;

  constructor(
    private readonly screen: Uint8ClampedArray,
    private readonly deps: PainterDeps,
  ) {
    this.from = new Uint8ClampedArray(screen.length);
    this.empty = new Uint8ClampedArray(screen.length);
  }

  /** Shows the art at `url` in `look` (no art without either), fading over `ms`; resolves once settled. */
  async show(url: string, look: BackdropLook | null, lookKey: string, ms: number): Promise<void> {
    const key = url && look ? `${lookKey}\n${url}` : '';
    // Any new target invalidates loads in flight, even one that returns to what is already drawn.
    const mine = ++this.generation;
    if (key === this.drawn) return;
    const to = key && look ? await this.picture(key, url, look) : this.empty;
    if (mine !== this.generation) return;
    this.drawn = key;
    if (to) this.fade(to, ms);
  }

  dispose() {
    this.generation++;
    this.deps.cancelFrame(this.frameId);
    this.frameId = 0;
    this.pictures.clear();
  }

  private picture(key: string, url: string, look: BackdropLook): Promise<Pixels | null> {
    let pending = this.pictures.get(key);
    if (!pending) {
      pending = this.deps.picture(url, look).catch((err: unknown) => {
        // A broken image is the art's problem, not the window's: no wash for it, and no retry.
        console.warn('backdrop: art could not be baked', url, err);
        return null;
      });
      this.pictures.set(key, pending);
      if (this.pictures.size > CACHE_SIZE) this.pictures.delete(this.pictures.keys().next().value as string);
    }
    return pending;
  }

  private fade(to: Pixels, ms: number) {
    this.deps.cancelFrame(this.frameId);
    this.frameId = 0;
    if (ms <= 0) {
      this.screen.set(to);
      this.deps.put();
      return;
    }
    // What is on screen now is where the fade starts, whether or not the last one finished.
    this.from.set(this.screen);
    const started = this.deps.now();
    const step = (now: number) => {
      const t = Math.min(1, Math.max(0, (now - started) / ms));
      mixPixels(this.from, to, t * t * (3 - 2 * t), this.screen);
      this.deps.put();
      this.frameId = t < 1 ? this.deps.frame(step) : 0;
    };
    this.frameId = this.deps.frame(step);
  }
}
