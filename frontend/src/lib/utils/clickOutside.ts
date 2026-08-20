export function clickOutside(node: HTMLElement, callback: () => void) {
  function onPointerDown(e: PointerEvent) {
    if (!node.contains(e.target as Node)) callback();
  }
  document.addEventListener('pointerdown', onPointerDown, true);
  return {
    destroy() {
      document.removeEventListener('pointerdown', onPointerDown, true);
    },
  };
}
