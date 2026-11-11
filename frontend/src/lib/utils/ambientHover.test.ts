import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

type Ambient = typeof import('../stores/ambient');
type Action = typeof import('./ambientHover');
let ambient: Ambient;
let action: Action;

beforeEach(async () => {
  vi.useFakeTimers();
  vi.resetModules();
  ambient = await import('../stores/ambient');
  action = await import('./ambientHover');
});

afterEach(() => {
  vi.useRealTimers();
});

function pointer(type: 'pointerenter' | 'pointerleave', pointerType = 'mouse'): Event {
  return Object.assign(new Event(type), { pointerType });
}

const target = () => ambient.pickAmbient(get(ambient.ambientState), null, () => true);
const rest = () => vi.advanceTimersByTime(ambient.HOVER_REST_MS);
const leaveTime = () => vi.advanceTimersByTime(ambient.HOVER_LEAVE_MS);

describe('ambient hover on a library card or row', () => {
  it('moves the backdrop to the game the pointer rests on and lets it go after leaving', () => {
    const node = new EventTarget() as HTMLElement;
    const handle = action.ambientHover(node, 'lib-1');
    node.dispatchEvent(pointer('pointerenter'));
    rest();
    expect(target()).toBe('lib-1');
    node.dispatchEvent(pointer('pointerleave'));
    leaveTime();
    expect(target()).toBeNull();
    handle.destroy();
  });

  it('ignores touch, and a card with no game', () => {
    const touched = new EventTarget() as HTMLElement;
    action.ambientHover(touched, 'lib-1');
    touched.dispatchEvent(pointer('pointerenter', 'touch'));
    const off = new EventTarget() as HTMLElement;
    action.ambientHover(off, null);
    off.dispatchEvent(pointer('pointerenter'));
    rest();
    expect(target()).toBeNull();
  });

  it('lets the game go when the card is removed under the pointer', () => {
    const node = new EventTarget() as HTMLElement;
    const handle = action.ambientHover(node, 'lib-1');
    node.dispatchEvent(pointer('pointerenter'));
    rest();
    handle.destroy();
    leaveTime();
    expect(target()).toBeNull();
    node.dispatchEvent(pointer('pointerenter'));
    rest();
    expect(target()).toBeNull();
  });

  it('follows a card that is reused for another game', () => {
    const node = new EventTarget() as HTMLElement;
    const handle = action.ambientHover(node, 'lib-1');
    node.dispatchEvent(pointer('pointerenter'));
    rest();
    handle.update('lib-2');
    leaveTime();
    expect(target()).toBeNull();
    node.dispatchEvent(pointer('pointerenter'));
    rest();
    expect(target()).toBe('lib-2');
  });
});
