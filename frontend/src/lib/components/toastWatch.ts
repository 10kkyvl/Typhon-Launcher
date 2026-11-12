/**
 * What the toast stack remembers about each toast while it is on screen: whether it is being read
 * (the pointer over it, focus inside it: either one stops its clock) and where focus was before it
 * entered the toast, to give it back when the toast goes. A toast leaves the list in several ways
 * (closed, timed out, removed by code), so the watch is told the list and not each way out: only a
 * toast on the list is remembered at all.
 */
export interface ToastWatch<E> {
  /**
   * The toasts on screen are now `present`. Everything kept about the others is dropped. If one of
   * those that left `holdsFocus`, where its focus came from is returned, for focus to go back there.
   */
  listed(present: readonly number[], holdsFocus: (id: number) => boolean): E | undefined;
  /** Records the change and says whether the toast is being read now. */
  read(id: number, change: { hover?: boolean; focus?: boolean }): boolean;
  /** Focus came into the toast from `from`. */
  entered(id: number, from: E): void;
  origin(id: number): E | undefined;
}

export function createToastWatch<E>(): ToastWatch<E> {
  let on = new Set<number>();
  const reading = new Map<number, { hover: boolean; focus: boolean }>();
  const origins = new Map<number, E>();

  return {
    listed(present, holdsFocus) {
      const gone = [...on].filter((id) => !present.includes(id));
      const held = gone.find(holdsFocus);
      const home = held === undefined ? undefined : origins.get(held);
      for (const id of gone) {
        reading.delete(id);
        origins.delete(id);
      }
      on = new Set(present);
      return home;
    },
    read(id, change) {
      if (!on.has(id)) return false;
      const state = { hover: false, focus: false, ...reading.get(id), ...change };
      reading.set(id, state);
      return state.hover || state.focus;
    },
    entered(id, from) {
      if (on.has(id)) origins.set(id, from);
    },
    origin: (id) => origins.get(id),
  };
}
