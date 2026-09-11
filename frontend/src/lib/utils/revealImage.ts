// A cached image may already be complete when the action is attached.
export function revealImage(node: HTMLImageElement) {
  const reveal = () => {
    if (node.naturalWidth > 0) node.dataset.ready = 'true';
  };
  node.addEventListener('load', reveal);
  if (node.complete) reveal();
  return {
    destroy() {
      node.removeEventListener('load', reveal);
    },
  };
}
