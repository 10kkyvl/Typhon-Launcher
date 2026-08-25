import { get } from 'svelte/store';
import { recallRoute, route, stashRoute } from '../stores/router';

const slot = 'scroll';
const maxFrames = 90;

export function scrollmemory(node: HTMLElement) {
  const key = get(route).key;
  const target = recallRoute<number>(key, slot) ?? 0;
  let restoring = target > 0;
  let frames = 0;
  let raf = 0;

  const save = () => {
    if (restoring || !node.isConnected) return;
    stashRoute(key, slot, node.scrollTop);
  };

  const restore = () => {
    node.scrollTop = target;
    if (Math.abs(node.scrollTop - target) < 1 || ++frames >= maxFrames) {
      restoring = false;
      return;
    }
    raf = requestAnimationFrame(restore);
  };

  const stop = () => {
    restoring = false;
    cancelAnimationFrame(raf);
  };

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
      node.removeEventListener('scroll', save);
      node.removeEventListener('wheel', stop);
      node.removeEventListener('pointerdown', stop);
      node.removeEventListener('keydown', stop);
    },
  };
}
