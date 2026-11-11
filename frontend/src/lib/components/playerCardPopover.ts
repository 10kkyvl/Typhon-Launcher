export interface Box { left: number; top: number; width: number; height: number }
export interface Size { width: number; height: number }

const clamp = (value: number, min: number, max: number) => Math.max(min, Math.min(max, value));

export function placePopover(anchor: Box, size: Size, viewport: Size, gap = 8, margin = 8): { left: number; top: number } {
  const maxLeft = Math.max(margin, viewport.width - margin - size.width);
  const maxTop = Math.max(margin, viewport.height - margin - size.height);
  const right = anchor.left + anchor.width + gap;
  if (right + size.width <= viewport.width - margin) {
    return { left: right, top: clamp(anchor.top, margin, maxTop) };
  }
  const left = anchor.left - gap - size.width;
  if (left >= margin) {
    return { left, top: clamp(anchor.top, margin, maxTop) };
  }
  const centred = clamp(anchor.left + anchor.width / 2 - size.width / 2, margin, maxLeft);
  const below = anchor.top + anchor.height + gap;
  if (below + size.height <= viewport.height - margin) return { left: centred, top: below };
  return { left: centred, top: clamp(anchor.top - gap - size.height, margin, maxTop) };
}
