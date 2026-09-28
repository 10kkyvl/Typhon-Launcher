interface Launch {
  confirmed: boolean;
  started: boolean;
  ended: boolean;
}

interface SessionOptions {
  launch: (id: string) => Promise<boolean>;
  onReturn: (id: string) => Promise<void>;
  onReturnError: (error: unknown) => void;
}

// The binding response and the session events may reach the UI in either order.
// Only a confirmed launch from this surface can bring the launcher to the front.
export function createBigPictureSession(options: SessionOptions) {
  const owned = new Map<string, Launch>();
  let disposed = false;

  function finish(id: string, launch: Launch) {
    if (disposed || !launch.confirmed || !launch.ended || owned.get(id) !== launch) return;
    owned.delete(id);
    void options.onReturn(id).catch((error: unknown) => {
      if (!disposed) options.onReturnError(error);
    });
  }

  return {
    async launch(id: string): Promise<boolean> {
      if (disposed || owned.has(id)) return false;
      const launch: Launch = { confirmed: false, started: false, ended: false };
      owned.set(id, launch);
      try {
        const accepted = await options.launch(id);
        if (!accepted || disposed) {
          owned.delete(id);
          return false;
        }
        launch.confirmed = true;
        finish(id, launch);
        return true;
      } catch (error) {
        owned.delete(id);
        throw error;
      }
    },
    observe(running: ReadonlySet<string>) {
      for (const [id, launch] of owned) {
        if (running.has(id)) launch.started = true;
        else if (launch.started) launch.ended = true;
        finish(id, launch);
      }
    },
    dispose() {
      disposed = true;
      owned.clear();
    },
  };
}
