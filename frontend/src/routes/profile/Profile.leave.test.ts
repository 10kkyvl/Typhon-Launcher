import { afterEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { confirmLeave, leaveRequest, navigate, registerLeaveGuard, resetHistory, route, stayHere } from '../../lib/stores/router';
import raw from './Profile.svelte?raw';

const source = raw.replace(/\r\n/g, '\n');

function harness(register: typeof registerLeaveGuard) {
  const start = source.indexOf('  $effect(() => {\n    if (!dirty) return;');
  const end = source.indexOf('\n  $effect(', start + 1);
  expect(start).toBeGreaterThan(-1);
  expect(end).toBeGreaterThan(start);
  const js = source.slice(start, end).replace('$effect(', 'effect(');
  const run = new Function(
    'registerLeaveGuard',
    'effect',
    'msg',
    `
    let dirty = false;
    ${js}
    return { setDirty: (value) => { dirty = value; } };
  `,
  );
  let rerun!: () => (() => void) | void;
  const api = run(register, (fn: () => (() => void) | void) => { rerun = fn; }, (key: string) => key) as {
    setDirty: (value: boolean) => void;
  };
  return { ...api, rerun: () => rerun() };
}

afterEach(() => {
  stayHere();
  resetHistory();
});

describe('profile leave guard', () => {
  it('registers nothing while the draft matches the saved profile', () => {
    const register = vi.fn(() => () => {});
    const h = harness(register);

    expect(h.rerun()).toBeUndefined();
    expect(register).not.toHaveBeenCalled();
  });

  it('registers a guard that reads the draft state live once it differs', () => {
    const register = vi.fn(() => () => {});
    const h = harness(register);
    h.setDirty(true);

    h.rerun();

    expect(register).toHaveBeenCalledTimes(1);
    const guard = (register.mock.calls[0] as unknown as [{ dirty: () => boolean; prompt: () => unknown }])[0];
    expect(guard.dirty()).toBe(true);
    h.setDirty(false);
    expect(guard.dirty()).toBe(false);
    expect(guard.prompt()).toEqual({
      title: 'profile.leaveTitle',
      text: 'profile.leaveText',
      confirm: 'profile.leaveConfirm',
      cancel: 'profile.leaveStay',
    });
  });

  it('removes the guard when the draft is saved or the page goes away', () => {
    const unregister = vi.fn();
    const h = harness(() => unregister);
    h.setDirty(true);

    const cleanup = h.rerun() as () => void;
    cleanup();

    expect(unregister).toHaveBeenCalledTimes(1);
  });

  it('asks before navigating away while the draft is unsaved and lets the user stay', () => {
    const h = harness(registerLeaveGuard);
    h.setDirty(true);
    const cleanup = h.rerun() as () => void;

    navigate('catalog');
    expect(get(route).name).toBe('library');
    expect(get(leaveRequest)?.prompt().title).toBe('profile.leaveTitle');

    stayHere();
    expect(get(route).name).toBe('library');

    navigate('catalog');
    confirmLeave();
    expect(get(route).name).toBe('catalog');

    cleanup();
    navigate('settings');
    expect(get(route).name).toBe('settings');
  });
});
