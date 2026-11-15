import { get } from 'svelte/store';
import { recallRoute, route, stashRoute } from '../stores/router';

const slot = 'scroll';
const maxFrames = 90;

/**
 * Keeps a route's scroll position for the way back and restores it over the frames a loading page needs
 * to grow. `onposition` hears where the page is as the user will see it settle: the position being
 * restored at once (the top for a new page), nothing while the restore is under way, the real position
 * when it ends, then the user's scrolling once a frame.
 */
export function scrollmemory(node: HTMLElement, onposition?: (top: number) => void) {
  const key = get(route).key;
  const target = recallRoute<number>(key, slot) ?? 0;
  let restoring = target > 0;
  let frames = 0;
  let raf = 0;
  let told = 0;

  const tell = () => {
    if (!onposition || told) return;
    told = requestAnimationFrame(() => {
      told = 0;
      onposition(node.scrollTop);
    });
  };

  const settle = () => {
    restoring = false;
    onposition?.(node.scrollTop);
  };

  const save = () => {
    if (restoring || !node.isConnected) return;
    stashRoute(key, slot, node.scrollTop);
    tell();
  };

  const restore = () => {
    node.scrollTop = target;
    if (Math.abs(node.scrollTop - target) < 1 || ++frames >= maxFrames) {
      settle();
      return;
    }
    raf = requestAnimationFrame(restore);
  };

  const stop = () => {
    cancelAnimationFrame(raf);
    if (restoring) settle();
  };

  onposition?.(target);
  if (restoring) {
    raf = requestAnimationFrame(restore);
    node.addEventListener('wheel', stop, { passive: true });
    node.addEventListener('pointerdown', stop, { passive: true });
    node.addEventListener('keydown', stop);
  }
  node.addEventListener('scroll', save, { passive: true });

  return {
    destroy() {
      cancelAnimationFrame(raf);
      cancelAnimationFrame(told);
      node.removeEventListener('scroll', save);
      node.removeEventListener('wheel', stop);
      node.removeEventListener('pointerdown', stop);
      node.removeEventListener('keydown', stop);
    },
  };
}
