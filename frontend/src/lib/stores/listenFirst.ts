import { Events } from '@wailsio/runtime';

type Handler = Parameters<typeof Events.On>[1];

/**
 * Ordering helper for store initialisation: subscriptions go up before the
 * first request, so an event fired while the request is in flight is not
 * lost and a rejected request still leaves the listeners attached. When an
 * event did arrive during the request, the snapshot is requested once more
 * so that it is at least as new as what the event described.
 */
export function listenFirst() {
  let seen = false;
  return {
    on(name: string, handler: Handler) {
      return Events.On(name, (event) => {
        seen = true;
        handler(event);
      });
    },
    async load<T>(request: () => Promise<T>): Promise<T> {
      const before = seen;
      let value = await request();
      if (seen && !before) value = await request();
      return value;
    },
  };
}
