const handlers = new WeakMap<Element, () => void>();
let observer: IntersectionObserver | undefined;

function shared(): IntersectionObserver {
  if (observer) return observer;
  observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        const handler = handlers.get(entry.target);
        if (!handler) continue;
        handlers.delete(entry.target);
        observer?.unobserve(entry.target);
        handler();
      }
    },
    { rootMargin: '600px 0px' },
  );
  return observer;
}

export function inview(node: HTMLElement, onenter: () => void) {
  handlers.set(node, onenter);
  shared().observe(node);
  return {
    destroy() {
      handlers.delete(node);
      observer?.unobserve(node);
    },
  };
}
