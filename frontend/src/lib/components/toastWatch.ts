/**
 * What the toast stack remembers about each toast while it is on screen: whether it is being read
 * (the pointer over it, focus inside it: either one stops its clock) and where focus was before it
 * entered the toast, to give it back when the toast goes. A toast leaves the list in several ways
 * (closed, timed out, removed by code), so the stack prunes against the list instead of trusting
 * every way out to clean up.
 */
export interface ToastWatch<E> {
  /** Records the change and says whether the toast is being read now. */
  read(id: number, change: { hover?: boolean; focus?: boolean }): boolean;
  /** Focus came into the toast from `from`. */
  entered(id: number, from: E): void;
  origin(id: number): E | undefined;
  forget(id: number): void;
  /** Forgets every toast not in `present` and returns their ids. */
  prune(present: Iterable<number>): number[];
  readonly size: number;
}

export function createToastWatch<E>(): ToastWatch<E> {
  const reading = new Map<number, { hover: boolean; focus: boolean }>();
  const origins = new Map<number, E>();

  const forget = (id: number) => {
    reading.delete(id);
    origins.delete(id);
  };

  return {
    read(id, change) {
      const state = { hover: false, focus: false, ...reading.get(id), ...change };
      const now = state.hover || state.focus;
      // Nothing is kept for a toast nobody reads, so a late event from a toast already gone leaves no entry.
      if (now) reading.set(id, state);
      else reading.delete(id);
      return now;
    },
    entered(id, from) {
      origins.set(id, from);
    },
    origin: (id) => origins.get(id),
    forget,
    prune(present) {
      const keep = new Set(present);
      const gone = [...new Set([...reading.keys(), ...origins.keys()])].filter((id) => !keep.has(id));
      gone.forEach(forget);
      return gone;
    },
    get size() {
      return new Set([...reading.keys(), ...origins.keys()]).size;
    },
  };
}
