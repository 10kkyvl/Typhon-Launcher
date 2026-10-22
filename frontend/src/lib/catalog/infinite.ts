export const maxAutoPages = 3;
export const sentinelMargin = 600;

export interface LoadMoreState {
  hasMore: boolean;
  loading: boolean;
  failed: boolean;
  pagesSinceScroll: number;
}

export function shouldLoadMore(state: LoadMoreState): boolean {
  return state.hasMore && !state.loading && !state.failed && state.pagesSinceScroll < maxAutoPages;
}

export function observerAvailable(): boolean {
  return typeof IntersectionObserver !== 'undefined';
}

function scrollParent(node: HTMLElement): HTMLElement | null {
  let parent = node.parentElement;
  while (parent && !/auto|scroll/.test(getComputedStyle(parent).overflowY)) parent = parent.parentElement;
  return parent;
}

// Fires once when the sentinel comes within the margin of the scroll area.
// The caller re-arms it after the load finishes, which re-reports visibility.
export function watchSentinel(node: HTMLElement, onVisible: () => void, margin = sentinelMargin): () => void {
  if (!observerAvailable()) return () => {};
  let fired = false;
  const observer = new IntersectionObserver(
    (entries) => {
      if (fired || !entries.some((entry) => entry.isIntersecting)) return;
      fired = true;
      observer.disconnect();
      onVisible();
    },
    { root: scrollParent(node), rootMargin: `0px 0px ${margin}px 0px` },
  );
  observer.observe(node);
  return () => observer.disconnect();
}
