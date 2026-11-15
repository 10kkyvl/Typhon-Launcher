/**
 * The arithmetic of reordering the downloads queue: from a key or a pointer to a place, the room the
 * other rows make during a drag, the order shown while moves are asked, and how repeated asks for one
 * download collapse into at most one `moveTo` in flight, and whether Go's order turned a download. No
 * DOM and no store: QueueList feeds it.
 *
 * A place is an index among the QUEUED downloads, the `position` Go's `MoveTo` takes.
 */

/** The place a key on a row's grip asks for (Arrow Up/Down one place, Home/End the ends); null for any other key. */
export function keyTarget(key: string, index: number, count: number): number | null {
  const last = Math.max(0, count - 1);
  switch (key) {
    case 'ArrowUp':
      return Math.max(0, index - 1);
    case 'ArrowDown':
      return Math.min(last, index + 1);
    case 'Home':
      return 0;
    case 'End':
      return last;
    default:
      return null;
  }
}

/** A row's box on the page as the drag measured it when it began. */
export interface RowBox {
  top: number;
  height: number;
}

const middle = (box: RowBox) => box.top + box.height / 2;

/**
 * Where the row at `from`, moved `dy` pixels by the pointer, would land: one place past every other
 * row whose middle its own middle has passed.
 */
export function pointerTarget(boxes: readonly RowBox[], from: number, dy: number): number {
  const centre = middle(boxes[from]) + dy;
  let target = 0;
  boxes.forEach((box, index) => {
    if (index !== from && middle(box) < centre) target += 1;
  });
  return target;
}

/** How far the row at `index` moves to make room while the row at `from` is held over `to`; `step` is the held row's size and gap. */
export function roomFor(index: number, from: number, to: number, step: number): number {
  if (index === from) return 0;
  if (from < index && index <= to) return -step;
  if (to <= index && index < from) return step;
  return 0;
}

/** `ids` with `id` taken out and put at `to` (clamped to the list); unchanged when `id` is not in it. */
export function placeAt(ids: readonly string[], id: string, to: number): string[] {
  const from = ids.indexOf(id);
  if (from < 0) return [...ids];
  const rest = ids.filter((other) => other !== id);
  rest.splice(Math.min(Math.max(0, to), rest.length), 0, id);
  return rest;
}

/** Go's order with every asked place applied in the order they were asked; an id no longer listed is skipped. */
export function shownOrder(ids: readonly string[], asked: ReadonlyMap<string, number>): string[] {
  let order = [...ids];
  for (const [id, to] of asked) order = placeAt(order, id, to);
  return order;
}

/**
 * Whether `id` stands among the other downloads in `after` otherwise than in `before`: the downloads
 * ahead of it changed, which only Go's reorder does. Downloads that came, went or changed their status
 * meanwhile count for nothing. Read over every download, not only the queued ones: a move that passes
 * a download which has since started is still a move.
 */
export function turned(before: readonly string[], after: readonly string[], id: string): boolean {
  const later = new Set(after);
  const kept = new Set(before.filter((other) => later.has(other)));
  if (!kept.has(id)) return false;
  const ahead = (ids: readonly string[]) => ids.slice(0, ids.indexOf(id)).filter((other) => kept.has(other));
  const was = new Set(ahead(before));
  const now = ahead(after);
  return now.length !== was.size || now.some((other) => !was.has(other));
}

/** The moves of one download: the place sent and not answered yet, and the last place asked meanwhile. */
export interface MoveAsk {
  sent: number;
  next: number | null;
}

export interface MoveStep {
  /** The download's moves after the step; undefined when none is in flight. */
  state: MoveAsk | undefined;
  /** The place to send now, null when nothing is sent. */
  send: number | null;
}

/**
 * The user asked for place `to`. With no move in flight it is sent; with one in flight it is kept, and
 * replaces any place kept before it: held keys and quick drags send one move at a time, not ten.
 */
export function askMove(state: MoveAsk | undefined, to: number): MoveStep {
  if (!state) return { state: { sent: to, next: null }, send: to };
  return { state: { sent: state.sent, next: to }, send: null };
}

/**
 * Go answered the move in flight. On success the place kept meanwhile is sent, unless it is the one
 * just made; on failure the kept place goes too, so the list falls back to Go's order instead of
 * trying a second move from a place Go did not confirm.
 */
export function settleMove(state: MoveAsk, ok: boolean): MoveStep {
  if (!ok || state.next === null || state.next === state.sent) return { state: undefined, send: null };
  return { state: { sent: state.next, next: null }, send: state.next };
}
