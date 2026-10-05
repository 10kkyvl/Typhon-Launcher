export interface ScrollFloorInput {
  scrollTop: number;
  viewport: number;
  scrollHeight: number;
  top: number;
  height: number;
}

export function scrollFloor(input: ScrollFloorInput): number {
  const trailing = Math.max(0, input.scrollHeight - input.top - input.height);
  return Math.max(0, Math.ceil(input.scrollTop + input.viewport - input.top - trailing));
}

export function measureScrollFloor(node: HTMLElement): number {
  const scroller = node.closest('main');
  if (!scroller) return 0;
  const top = node.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop;
  return scrollFloor({
    scrollTop: scroller.scrollTop,
    viewport: scroller.clientHeight,
    scrollHeight: scroller.scrollHeight,
    top,
    height: node.offsetHeight,
  });
}
