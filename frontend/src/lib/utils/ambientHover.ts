import { hoverAmbient } from '../stores/ambient';

/** Lets the window backdrop follow the game of a library card or row the pointer rests on; `null` opts out. */
export function ambientHover(node: HTMLElement, gameId: string | null) {
  let id = gameId;
  let release: (() => void) | null = null;
  const leave = () => {
    release?.();
    release = null;
  };
  const enter = (event: Event) => {
    if (!id || (event as PointerEvent).pointerType === 'touch') return;
    leave();
    release = hoverAmbient(id);
  };
  node.addEventListener('pointerenter', enter);
  node.addEventListener('pointerleave', leave);
  return {
    update(next: string | null) {
      if (next === id) return;
      leave();
      id = next;
    },
    destroy() {
      leave();
      node.removeEventListener('pointerenter', enter);
      node.removeEventListener('pointerleave', leave);
    },
  };
}
