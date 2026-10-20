export const notesDelay = 800;

export function noteLength(text: string): number {
  return [...text].length;
}

export interface Autosave {
  schedule(text: string): void;
  flush(): Promise<void>;
  pending(): boolean;
}

export function createAutosave(save: (text: string) => Promise<void>, delay = notesDelay): Autosave {
  let timer: ReturnType<typeof setTimeout> | null = null;
  let queued: string | null = null;
  let chain: Promise<void> = Promise.resolve();

  function stop() {
    if (timer !== null) clearTimeout(timer);
    timer = null;
  }

  function flush(): Promise<void> {
    stop();
    if (queued === null) return chain;
    const text = queued;
    queued = null;
    const run = () =>
      save(text).catch((err: unknown) => {
        if (queued === null) queued = text;
        throw err;
      });
    chain = chain.then(run, run);
    return chain;
  }

  return {
    schedule(text) {
      queued = text;
      stop();
      timer = setTimeout(() => {
        flush().catch(() => undefined);
      }, delay);
    },
    flush,
    pending: () => queued !== null,
  };
}
