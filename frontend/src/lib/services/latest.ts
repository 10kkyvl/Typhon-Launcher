// createLatest guards a load that can be started again before the previous
// one finished -- reopening a modal, retyping a query. Each start() hands
// back a predicate that reports whether its own attempt is still the newest,
// so a slow earlier response cannot overwrite the state a later one already
// applied.
export function createLatest() {
  let current = 0;
  return {
    start(): () => boolean {
      const mine = ++current;
      return () => mine === current;
    },
  };
}
