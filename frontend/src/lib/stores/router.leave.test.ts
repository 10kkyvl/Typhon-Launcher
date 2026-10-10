import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import {
  canGoBack,
  canGoForward,
  confirmLeave,
  goBack,
  goForward,
  leaveRequest,
  navigate,
  registerLeaveGuard,
  resetHistory,
  route,
  stayHere,
} from './router';

const prompt = { title: 'Leave?', text: 'Unsaved', confirm: 'Leave', cancel: 'Stay' };

let unregister: (() => void) | null = null;
let unsaved = true;

function guard() {
  unregister = registerLeaveGuard({ dirty: () => unsaved, prompt: () => prompt });
}

beforeEach(() => {
  unsaved = true;
  resetHistory();
});

afterEach(() => {
  unregister?.();
  unregister = null;
  stayHere();
});

describe('router leave guard', () => {
  it('navigates immediately when no guard is registered', () => {
    navigate('catalog');
    expect(get(route).name).toBe('catalog');
    expect(get(leaveRequest)).toBeNull();
  });

  it('navigates immediately when the guard reports nothing unsaved', () => {
    guard();
    unsaved = false;
    navigate('catalog');
    expect(get(route).name).toBe('catalog');
    expect(get(leaveRequest)).toBeNull();
  });

  it('holds navigate until the user confirms', () => {
    guard();
    navigate('catalog');
    expect(get(route).name).toBe('library');
    expect(get(leaveRequest)?.prompt()).toEqual(prompt);

    confirmLeave();
    expect(get(route).name).toBe('catalog');
    expect(get(leaveRequest)).toBeNull();
  });

  it('drops navigate and leaves history untouched when the user stays', () => {
    guard();
    navigate('catalog');
    stayHere();

    expect(get(route).name).toBe('library');
    expect(get(canGoBack)).toBe(false);
    expect(get(canGoForward)).toBe(false);
    expect(get(leaveRequest)).toBeNull();
  });

  it('does not ask for a navigation to the page already open', () => {
    guard();
    navigate('library');
    expect(get(leaveRequest)).toBeNull();
  });

  it('holds goBack until the user confirms', () => {
    navigate('catalog');
    guard();

    goBack();
    expect(get(route).name).toBe('catalog');
    expect(get(leaveRequest)).not.toBeNull();

    confirmLeave();
    expect(get(route).name).toBe('library');
    expect(get(canGoForward)).toBe(true);
  });

  it('keeps the position when the user stays on goBack', () => {
    navigate('catalog');
    guard();

    goBack();
    stayHere();

    expect(get(route).name).toBe('catalog');
    expect(get(canGoBack)).toBe(true);
    expect(get(canGoForward)).toBe(false);
  });

  it('holds goForward until the user confirms', () => {
    navigate('catalog');
    goBack();
    guard();

    goForward();
    expect(get(route).name).toBe('library');
    expect(get(leaveRequest)).not.toBeNull();

    confirmLeave();
    expect(get(route).name).toBe('catalog');
  });

  it('keeps the position when the user stays on goForward', () => {
    navigate('catalog');
    goBack();
    guard();

    goForward();
    stayHere();

    expect(get(route).name).toBe('library');
    expect(get(canGoForward)).toBe(true);
  });

  it('does not ask when there is nowhere to go back or forward', () => {
    guard();
    goBack();
    goForward();
    expect(get(leaveRequest)).toBeNull();
  });

  it('opens one question for repeated clicks and ignores the later targets', () => {
    guard();
    navigate('catalog');
    const first = get(leaveRequest);
    navigate('settings');
    goBack();
    goForward();

    expect(get(leaveRequest)).toBe(first);
    confirmLeave();
    expect(get(route).name).toBe('catalog');
    confirmLeave();
    expect(get(route).name).toBe('catalog');
  });

  it('asks again for the next navigation after the user stayed', () => {
    guard();
    navigate('catalog');
    stayHere();
    navigate('settings');

    expect(get(leaveRequest)).not.toBeNull();
    confirmLeave();
    expect(get(route).name).toBe('settings');
  });

  it('stops asking once the guard is unregistered', () => {
    guard();
    unregister?.();
    unregister = null;

    navigate('catalog');
    expect(get(route).name).toBe('catalog');
    expect(get(leaveRequest)).toBeNull();
  });

  it('keeps a newer guard when an older registration is removed late', () => {
    const stale = registerLeaveGuard({ dirty: () => false, prompt: () => prompt });
    guard();
    stale();

    navigate('catalog');
    expect(get(leaveRequest)).not.toBeNull();
  });

  it('reads the unsaved state at navigation time', () => {
    const dirty = vi.fn(() => false);
    unregister = registerLeaveGuard({ dirty, prompt: () => prompt });

    navigate('catalog');
    expect(get(route).name).toBe('catalog');

    dirty.mockReturnValue(true);
    navigate('settings');
    expect(get(route).name).toBe('catalog');
    expect(get(leaveRequest)).not.toBeNull();
  });

  it('resets history without asking and drops a pending question', () => {
    guard();
    navigate('catalog');
    expect(get(leaveRequest)).not.toBeNull();

    resetHistory();
    expect(get(leaveRequest)).toBeNull();
    expect(get(route).name).toBe('library');
  });

  it('goes on with the held navigation once the changes got saved while the question was open', () => {
    guard();
    navigate('catalog');
    expect(get(leaveRequest)).not.toBeNull();

    unsaved = false;
    unregister?.();
    unregister = null;

    expect(get(leaveRequest)).toBeNull();
    expect(get(route).name).toBe('catalog');
  });

  it('keeps the question when the page unmounts with the changes still unsaved', () => {
    guard();
    navigate('catalog');

    unregister?.();
    unregister = null;

    expect(get(leaveRequest)).not.toBeNull();
    expect(get(route).name).toBe('library');
  });

  it('does nothing when asked to confirm with no question open', () => {
    confirmLeave();
    stayHere();
    expect(get(route).name).toBe('library');
  });
});
