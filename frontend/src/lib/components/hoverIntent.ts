export const HOVER_DELAY = 350;

export interface HoverIntent {
  hover(): void;
  unhover(): void;
  focus(): void;
  blur(): void;
  escape(): void;
  dismiss(): void;
  destroy(): void;
  readonly open: boolean;
}

export function createHoverIntent(onchange: (open: boolean) => void, delay = HOVER_DELAY): HoverIntent {
  let timer: ReturnType<typeof setTimeout> | null = null;
  let hovered = false;
  let focused = false;
  let dismissed = false;
  let open = false;

  const cancel = () => {
    if (timer !== null) { clearTimeout(timer); timer = null; }
  };
  const update = () => {
    const next = (hovered || focused) && !dismissed;
    if (next === open) return;
    open = next;
    onchange(next);
  };
  const settle = () => {
    if (!hovered && !focused) dismissed = false;
    update();
  };

  return {
    hover() {
      if (hovered || timer !== null) return;
      timer = setTimeout(() => { timer = null; hovered = true; update(); }, delay);
    },
    unhover() {
      cancel();
      hovered = false;
      settle();
    },
    focus() {
      focused = true;
      update();
    },
    blur() {
      focused = false;
      settle();
    },
    escape() {
      dismissed = true;
      update();
    },
    dismiss() {
      cancel();
      hovered = false;
      dismissed = true;
      update();
      dismissed = false;
    },
    destroy() {
      cancel();
    },
    get open() {
      return open;
    },
  };
}
