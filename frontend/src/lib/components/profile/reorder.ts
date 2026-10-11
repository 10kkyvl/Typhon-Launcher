export interface Box {
  index: number;
  left: number;
  top: number;
  right: number;
  bottom: number;
}

export function pickTarget(x: number, y: number, boxes: Box[]): number | null {
  if (boxes.length === 0) return null;
  const inside = boxes.find((box) => x >= box.left && x <= box.right && y >= box.top && y <= box.bottom);
  if (inside) return inside.index;
  let best = boxes[0];
  let bestDistance = Infinity;
  for (const box of boxes) {
    const dx = (box.left + box.right) / 2 - x;
    const dy = (box.top + box.bottom) / 2 - y;
    const distance = dx * dx + dy * dy;
    if (distance < bestDistance) {
      best = box;
      bestDistance = distance;
    }
  }
  return best.index;
}
