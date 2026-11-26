import { render } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { loadHandoff } from './lib/stores/handoff';
import { authState } from './lib/stores/user';
import App from './App.svelte';

// The successor of a seamless restart, seen at the edge to Go: every store and service is the real
// one, and the runtime records every bound method the window calls. Go holds a successor's services
// back (handoffgate.Startup) until the old launcher has exited and only then sends
// launcher:handoff_done; a held service answers as if it had never started, social with an error
// outside the account contract. So nothing may reach one before that event: not the stores, not their
// reactions to a settled sign-in, not the shell that mounts under the curtain once it has settled.

const calls = vi.hoisted(() => [] as number[]);
const handlers = vi.hoisted(() => new Map<string, Array<(event: { data: unknown }) => void>>());
const answers = vi.hoisted(() => new Map<number, unknown>());
const mapping = vi.hoisted(() => ({ on: false, last: -1 }));

vi.mock('@wailsio/runtime', async () => {
  const runtime = (await import('./lib/testing/dom')).wailsRuntime();
  runtime.Events.On = vi.fn((name: string | undefined, cb: (event: unknown) => void) => {
    if (!name) return () => {};
    handlers.set(name, [...(handlers.get(name) ?? []), cb as (event: { data: unknown }) => void]);
    return () => handlers.set(name, (handlers.get(name) ?? []).filter((other) => other !== cb));
  });
  return {
    ...runtime,
    Call: {
      ByID: vi.fn((id: number) => {
        if (mapping.on) mapping.last = id;
        else calls.push(id);
        return Promise.resolve(answers.has(id) ? answers.get(id) : null);
      }),
    },
  };
});
vi.mock('./lib/services/backend', () => ({ inWails: true }));

const bindings = import.meta.glob('../bindings/typhon/**/*.ts', { eager: true }) as Record<string, Record<string, unknown>>;

/** Bound method id → `package.Method`: each generated binding is called once with the recorder on. */
function boundMethods(): Map<number, string> {
  const methods = new Map<number, string>();
  mapping.on = true;
  try {
    for (const [path, module] of Object.entries(bindings)) {
      const file = /bindings\/typhon\/(?:internal\/)?(.+)\.ts$/.exec(path)?.[1] ?? path;
      if (/(^|\/)(index|models)$/.test(file)) continue;
      const pkg = file.includes('/') ? file.slice(0, file.lastIndexOf('/')) : file;
      for (const [name, value] of Object.entries(module)) {
        if (typeof value !== 'function') continue;
        mapping.last = -1;
        try {
          void (value as () => Promise<unknown>)().catch(() => {});
        } catch {
          // A binding that checks its arguments before calling Go is not one an init reaches bare.
          continue;
        }
        if (mapping.last >= 0) methods.set(mapping.last, `${pkg}.${name}`);
      }
    }
  } finally {
    mapping.on = false;
  }
  return methods;
}

// `grep -rn 'handoffgate.Startup' internal/`: the bound services whose startup a successor holds back.
// compat's held part is its Sharer, which binds nothing.
const HELD = new Set([
  'accountsync', 'achievementsync', 'cloudsaves', 'diagnostics', 'discord', 'download', 'extlib', 'gamemode',
  'heartbeat', 'install', 'lan', 'library', 'media', 'metadata', 'online', 'overlay', 'relocate', 'savebackup',
  'social', 'sources', 'titlesdict', 'updates', 'usagestats', 'wrappers',
]);
// Set as the metadata service module loads: a field in memory, no file, no error, needed before
// metadata's own startup runs after the gate.
const AT_IMPORT = new Set(['metadata.SetLanguage']);

const ACTIVE = { active: true, phaseStart: Date.now(), version: '1.5.0', background: '#101418' };
const USER = { id: 'u1', username: 'player', displayName: 'Player', email: 'p@example.test', avatarUrl: '' };

beforeAll(() => {
  Object.defineProperty(Element.prototype, 'animate', {
    value: () => ({
      cancel: () => {},
      finished: Promise.resolve(),
      set onfinish(done: () => void) {
        done();
      },
    }),
    configurable: true,
    writable: true,
  });
});

afterAll(() => {
  Reflect.deleteProperty(Element.prototype, 'animate');
});

function named(ids: number[], methods: Map<number, string>): string[] {
  return ids.map((id) => methods.get(id) ?? `unknown.${id}`);
}

function held(names: string[]): string[] {
  return names.filter((name) => HELD.has(name.split('.')[0]) && !AT_IMPORT.has(name));
}

describe('a successor of a seamless restart, before the old launcher is gone', () => {
  it('calls no held-back service, with the shell up under the curtain, and calls them once Go lets them start', async () => {
    const methods = boundMethods();
    const id = (name: string) => [...methods].find(([, method]) => method === name)?.[0] ?? -1;
    expect(id('selfupdate.HandoffInfo')).toBeGreaterThan(-1);
    expect(id('social.Friends')).toBeGreaterThan(-1);
    answers.set(id('selfupdate.HandoffInfo'), ACTIVE);
    answers.set(id('account.Bootstrap'), { status: 'authenticated', user: USER, reason: '' });
    // Answers Go never gives as null: one would throw in a store subscriber and stop every store.
    answers.set(id('selfupdate.GetStatus'), { state: 'idle', currentVersion: '1.5.0' });
    answers.set(id('navigationservice.TakePendingLink'), { query: '', invalid: false });

    await loadHandoff();
    const view = render(App);
    await vi.waitFor(() => expect(get(authState)).toBe('authenticated'));
    // Let every answer already asked for land, and what it sets off ask in turn.
    for (let i = 0; i < 20; i++) await tick();

    expect(document.querySelector('nav')).not.toBeNull();
    expect(held(named(calls, methods))).toEqual([]);

    const before = calls.length;
    for (const handler of [...(handlers.get('launcher:handoff_done') ?? [])]) handler({ data: { ...ACTIVE, active: false } });
    await vi.waitFor(() => expect(named(calls.slice(before), methods)).toContain('social.Kick'));
    expect(named(calls.slice(before), methods)).toEqual(expect.arrayContaining(['download.List', 'install.List', 'library.GetGames', 'social.Kick', 'online.Kick']));
    view.unmount();
  }, 60_000);
});
