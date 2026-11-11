/**
 * What the toast stack remembers about each toast while it is on screen: whether it is being read
 * (the pointer over it, focus inside it: either one stops its clock) and where focus was before it
 * entered the toast, to give it back when the toast goes.
 */
export interface ToastWatch<E> {
  /** Records the change and says whether the toast is being read now. */
  read(id: number, change: { hover?: boolean; focus?: boolean }): boolean;
  /** Focus came into the toast from `from`. */
  entered(id: number, from: E): void;
  origin(id: number): E | undefined;
  forget(id: number): void;
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
      reading.set(id, state);
      return state.hover || state.focus;
    },
    entered(id, from) {
      origins.set(id, from);
    },
    origin: (id) => origins.get(id),
    forget,
  };
}
