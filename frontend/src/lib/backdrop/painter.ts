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

/** Baked pictures kept for going back and forth between games, least recently used out first; each is a few kilobytes. */
export const PICTURE_CACHE_SIZE = 32;
/** A picture that has neither loaded nor failed by then counts as failed. */
export const PICTURE_TIMEOUT_MS = 10_000;

/**
 * Draws the backdrop into one small buffer: a picture per art URL and look, baked once, and a
 * cross-fade from the picture on screen to the next one. A game's pictures are tried in order, each
 * once per look; when none works the backdrop fades to the plain page, never keeping another game's
 * or another theme's pixels. Only the newest target is ever drawn, and frames are requested only
 * while a fade runs.
 */
export class BackdropPainter {
  private readonly pictures = new Map<string, Promise<Pixels | null>>();
  /** Stall timers of pictures still loading, each with the way to drop it quietly. */
  private readonly stalls = new Set<() => void>();
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

  /** Shows the first of `sources` that bakes in `look` (none without a look), fading over `ms`. */
  async show(sources: readonly string[], look: BackdropLook | null, lookKey: string, ms: number): Promise<void> {
    const key = sources.length > 0 && look ? [lookKey, ...sources].join('\n') : '';
    // Any new target invalidates loads in flight, even one that returns to what is already drawn.
    const mine = ++this.generation;
    if (key === this.drawn) return;
    let to: Pixels | null = null;
    if (look) {
      for (const url of sources) {
        to = await this.picture(`${lookKey}\n${url}`, url, look);
        if (mine !== this.generation) return;
        if (to) break;
      }
    }
    this.drawn = key;
    this.fade(to ?? this.empty, ms);
  }

  dispose() {
    this.generation++;
    this.deps.cancelFrame(this.frameId);
    this.frameId = 0;
    this.pictures.clear();
    for (const drop of [...this.stalls]) drop();
  }

  private picture(key: string, url: string, look: BackdropLook): Promise<Pixels | null> {
    const cached = this.pictures.get(key);
    if (cached) {
      // A hit makes the picture the most recently used.
      this.pictures.delete(key);
      this.pictures.set(key, cached);
      return cached;
    }
    const pending = this.settle(this.deps.picture(url, look), url).then(({ pixels, stalled }) => {
      // A stall may be the network, not the art: the next showing tries again. A failure is the art's.
      if (stalled && this.pictures.get(key) === pending) this.pictures.delete(key);
      return pixels;
    });
    this.pictures.set(key, pending);
    if (this.pictures.size > PICTURE_CACHE_SIZE) this.pictures.delete(this.pictures.keys().next().value as string);
    return pending;
  }

  /** The picture, or null once it fails, rejects or stalls past PICTURE_TIMEOUT_MS. */
  private settle(picture: Promise<Pixels | null>, url: string): Promise<{ pixels: Pixels | null; stalled: boolean }> {
    return new Promise((resolve) => {
      const done = (pixels: Pixels | null, stalled: boolean) => {
        clearTimeout(timer);
        this.stalls.delete(drop);
        resolve({ pixels, stalled });
      };
      const drop = () => done(null, true);
      const timer = setTimeout(() => {
        console.warn('backdrop: art took too long to load', url);
        done(null, true);
      }, PICTURE_TIMEOUT_MS);
      this.stalls.add(drop);
      picture.then(
        (pixels) => done(pixels, false),
        (err: unknown) => {
          // A broken image is the art's problem, not the window's: no wash from it, and no retry.
          console.warn('backdrop: art could not be baked', url, err);
          done(null, false);
        },
      );
    });
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
