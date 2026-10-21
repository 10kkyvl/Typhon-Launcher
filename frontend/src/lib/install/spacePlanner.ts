import type { SpaceInfo } from '../services/install';

export interface SpaceRequest {
  downloadId: string;
  destination: string;
  mode: string;
}

export interface SpacePlannerHandlers {
  fetch: (downloadId: string, destination: string, mode: string) => Promise<SpaceInfo>;
  onPending: () => void;
  onResult: (info: SpaceInfo) => void;
  onError: (err: unknown) => void;
}

export const spaceDebounceMs = 400;

export function createSpacePlanner(handlers: SpacePlannerHandlers, delay: number = spaceDebounceMs) {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let token = 0;

  function run(request: SpaceRequest) {
    const current = ++token;
    handlers.onPending();
    handlers
      .fetch(request.downloadId, request.destination, request.mode)
      .then((info) => {
        if (current === token) handlers.onResult(info);
      })
      .catch((err) => {
        if (current === token) handlers.onError(err);
      });
  }

  return {
    schedule(request: SpaceRequest) {
      clearTimeout(timer);
      token++;
      handlers.onPending();
      timer = setTimeout(() => run(request), delay);
    },
    now(request: SpaceRequest) {
      clearTimeout(timer);
      run(request);
    },
    cancel() {
      clearTimeout(timer);
      token++;
    },
  };
}
