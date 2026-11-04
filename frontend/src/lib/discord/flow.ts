import { writable, type Readable } from 'svelte/store';
import type { DiscordApi, DiscordLink } from '../services/discord';

export type DiscordPhase = 'loading' | 'unlinked' | 'code' | 'linked' | 'unavailable' | 'error';

export interface DiscordState {
  phase: DiscordPhase;
  /** Raw code while phase is 'code'. */
  code: string;
  /** Seconds until the code expires while phase is 'code'. */
  remaining: number;
  link: DiscordLink | null;
  /** The last code ran out before the player used it. */
  expired: boolean;
  busy: boolean;
  /** Account error code of the last failed action, or ''. */
  errorCode: string;
}

export interface DiscordFlow {
  state: Readable<DiscordState>;
  load(): Promise<void>;
  start(): Promise<void>;
  unlink(): Promise<void>;
  /** Stops polling and ignores replies still in flight. */
  dispose(): void;
}

export interface DiscordFlowOptions {
  pollMs?: number;
  now?: () => number;
  /** Called when the session is gone, so the caller can run its re-login path. */
  onUnauthenticated?: () => void;
}

const initial: DiscordState = {
  phase: 'loading',
  code: '',
  remaining: 0,
  link: null,
  expired: false,
  busy: false,
  errorCode: '',
};

function codeOf(err: unknown): string {
  const code = (err as { code?: unknown } | null)?.code;
  return typeof code === 'string' ? code : 'server_error';
}

export function createDiscordFlow(api: DiscordApi, options: DiscordFlowOptions = {}): DiscordFlow {
  const pollMs = options.pollMs ?? 3000;
  const now = options.now ?? Date.now;
  const store = writable<DiscordState>({ ...initial });
  let current = { ...initial };
  let disposed = false;
  let epoch = 0;
  let expiresAt = 0;
  let poll: ReturnType<typeof setInterval> | undefined;
  let tick: ReturnType<typeof setInterval> | undefined;
  let polling = false;

  function set(patch: Partial<DiscordState>) {
    current = { ...current, ...patch };
    store.set(current);
  }

  function stopTimers() {
    if (poll !== undefined) clearInterval(poll);
    if (tick !== undefined) clearInterval(tick);
    poll = tick = undefined;
    polling = false;
  }

  function fail(err: unknown, mine: number): boolean {
    if (mine !== epoch || disposed) return true;
    const code = codeOf(err);
    if (code === 'discord_unavailable') {
      stopTimers();
      set({ phase: 'unavailable', busy: false, errorCode: '', code: '' });
      return true;
    }
    if (code === 'unauthenticated') {
      stopTimers();
      set({ busy: false });
      options.onUnauthenticated?.();
      return true;
    }
    return false;
  }

  function applyLink(link: DiscordLink) {
    stopTimers();
    if (link.linked) set({ phase: 'linked', link, code: '', remaining: 0, expired: false, busy: false, errorCode: '' });
    else set({ phase: 'unlinked', link: null, code: '', remaining: 0, busy: false, errorCode: '' });
  }

  async function load() {
    const mine = ++epoch;
    stopTimers();
    set({ phase: 'loading', errorCode: '' });
    try {
      const link = await api.getLink();
      if (mine !== epoch || disposed) return;
      applyLink(link);
    } catch (err) {
      if (fail(err, mine)) return;
      set({ phase: 'error', errorCode: codeOf(err) });
    }
  }

  async function pollOnce(mine: number) {
    if (polling || mine !== epoch || disposed) return;
    polling = true;
    try {
      const link = await api.getLink();
      if (mine !== epoch || disposed) return;
      if (link.linked) applyLink(link);
    } catch (err) {
      // A dropped poll is not the end of the flow: the next one may succeed.
      fail(err, mine);
    } finally {
      polling = false;
    }
  }

  function onTick(mine: number) {
    if (mine !== epoch || disposed) return;
    const remaining = Math.max(0, Math.ceil((expiresAt - now()) / 1000));
    if (remaining > 0) {
      set({ remaining });
      return;
    }
    stopTimers();
    set({ phase: 'unlinked', code: '', remaining: 0, expired: true });
  }

  async function start() {
    if (current.busy) return;
    const mine = ++epoch;
    stopTimers();
    set({ busy: true, errorCode: '', expired: false });
    try {
      const issued = await api.createLinkCode();
      if (mine !== epoch || disposed) return;
      expiresAt = new Date(issued.expiresAt).getTime();
      const remaining = Math.max(0, Math.ceil((expiresAt - now()) / 1000));
      if (!issued.code || !Number.isFinite(expiresAt) || remaining === 0) {
        set({ busy: false, errorCode: 'server_error' });
        return;
      }
      set({ phase: 'code', code: issued.code, remaining, busy: false });
      poll = setInterval(() => void pollOnce(mine), pollMs);
      tick = setInterval(() => onTick(mine), 1000);
    } catch (err) {
      if (fail(err, mine)) return;
      set({ busy: false, errorCode: codeOf(err) });
    }
  }

  async function unlink() {
    if (current.busy) return;
    const mine = ++epoch;
    set({ busy: true, errorCode: '' });
    try {
      await api.unlink();
      if (mine !== epoch || disposed) return;
      applyLink({ linked: false, discordId: '', discordUsername: '', linkedAt: '' });
    } catch (err) {
      if (fail(err, mine)) return;
      set({ busy: false, errorCode: codeOf(err) });
    }
  }

  function dispose() {
    disposed = true;
    epoch++;
    stopTimers();
  }

  return { state: { subscribe: store.subscribe }, load, start, unlink, dispose };
}
