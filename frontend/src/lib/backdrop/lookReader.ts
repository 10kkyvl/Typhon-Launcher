import type { BackdropLook } from './pixels';

/**
 * Reads the backdrop look from the window at most once per animation frame. A live theme trial moves the
 * theme on every input of a colour picker; reading (a probe element, computed styles, maybe a bake) on
 * each would stall the editor. The read at the frame sees the latest state, and a look equal to the
 * last one is not passed on, so nothing downstream rebakes or restarts a fade.
 */
export class LookReader {
  private frameId = 0;
  private key: string | null = null;
  private disposed = false;

  constructor(
    private readonly read: () => BackdropLook | null,
    private readonly frame: (step: () => void) => number,
    private readonly cancel: (id: number) => void,
    private readonly apply: (look: BackdropLook | null, key: string) => void,
  ) {}

  request() {
    if (this.frameId || this.disposed) return;
    this.frameId = this.frame(() => {
      this.frameId = 0;
      const look = this.read();
      const key = look ? JSON.stringify(look) : '';
      if (key === this.key) return;
      this.key = key;
      this.apply(look, key);
    });
  }

  dispose() {
    this.disposed = true;
    this.cancel(this.frameId);
    this.frameId = 0;
  }
}
