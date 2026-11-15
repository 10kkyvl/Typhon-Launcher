/**
 * Puts the items of `list` into the order `order` gives their ids, and changes nothing else: an item
 * `order` does not name keeps its place, an id `order` names that `list` lacks is ignored, and every
 * item is the one `list` held. Go's queue order then comes from a snapshot, while the records and the
 * set of downloads keep coming from the events, which may be newer than that snapshot.
 */
export function followOrder<T extends { id: string }>(list: readonly T[], order: readonly string[]): T[] {
  const rank = new Map(order.map((id, index) => [id, index]));
  const slots: number[] = [];
  list.forEach((item, index) => {
    if (rank.has(item.id)) slots.push(index);
  });
  const placed = slots.map((index) => list[index]).sort((a, b) => (rank.get(a.id) ?? 0) - (rank.get(b.id) ?? 0));
  const next = [...list];
  slots.forEach((slot, k) => {
    next[slot] = placed[k];
  });
  return next;
}
