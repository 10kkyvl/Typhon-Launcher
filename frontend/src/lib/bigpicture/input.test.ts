import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  BIG_PICTURE_DIRECTION_REPEAT_DELAY_MS,
  BIG_PICTURE_DIRECTION_REPEAT_INTERVAL_MS,
  startBigPictureInput,
  type BigPictureCommand,
  type BigPictureDevice,
} from './input';

type Listener = (event: Record<string, unknown>) => void;

function eventTarget() {
  const listeners = new Map<string, Set<Listener>>();
  return {
    addEventListener(type: string, listener: Listener) {
      const values = listeners.get(type) ?? new Set<Listener>();
      values.add(listener);
      listeners.set(type, values);
    },
    removeEventListener(type: string, listener: Listener) {
      listeners.get(type)?.delete(listener);
    },
    dispatch(type: string, event: Record<string, unknown> = {}) {
      for (const listener of [...(listeners.get(type) ?? [])]) listener(event);
    },
  };
}

interface FakePad {
  id: string;
  index: number;
  mapping: string;
  connected: boolean;
  axes: number[];
  buttons: Array<{ pressed: boolean; value: number }>;
}

function gamepad(id = 'Xbox Wireless Controller'): FakePad {
  return {
    id,
    index: 0,
    mapping: 'standard',
    connected: true,
    axes: [0, 0],
    buttons: Array.from({ length: 16 }, () => ({ pressed: false, value: 0 })),
  };
}

function keyEvent(key: string, options: Record<string, unknown> = {}) {
  let prevented = false;
  return {
    key,
    code: options.code ?? key,
    target: options.target ?? null,
    repeat: options.repeat ?? false,
    ctrlKey: options.ctrlKey ?? false,
    metaKey: options.metaKey ?? false,
    altKey: options.altKey ?? false,
    preventDefault() {
      prevented = true;
    },
    wasPrevented() {
      return prevented;
    },
  };
}

describe('startBigPictureInput', () => {
  let browserWindow: ReturnType<typeof eventTarget>;
  let browserDocument: ReturnType<typeof eventTarget> & {
    hidden: boolean;
    visibilityState: 'visible' | 'hidden';
    hasFocus: () => boolean;
  };
  let pads: FakePad[];

  beforeEach(() => {
    vi.useFakeTimers();
    browserWindow = eventTarget();
    browserDocument = Object.assign(eventTarget(), {
      hidden: false,
      visibilityState: 'visible' as const,
      hasFocus: () => true,
    });
    pads = [];
    vi.stubGlobal('window', browserWindow);
    vi.stubGlobal('document', browserDocument);
    vi.stubGlobal('navigator', { getGamepads: () => pads });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('maps keyboard controls, prevents accepted browser defaults, and ignores text inputs/system shortcuts', () => {
    const commands: BigPictureCommand[] = [];
    const stop = startBigPictureInput({ onCommand: (command) => commands.push(command), isEnabled: () => true });

    const arrow = keyEvent('ArrowRight');
    browserWindow.dispatch('keydown', arrow);
    browserWindow.dispatch('keyup', arrow);
    const q = keyEvent('й', { code: 'KeyQ' });
    browserWindow.dispatch('keydown', q);
    browserWindow.dispatch('keyup', q);
    const ctrlQ = keyEvent('q', { ctrlKey: true });
    browserWindow.dispatch('keydown', ctrlQ);
    const textInput = keyEvent('Enter', { target: { tagName: 'INPUT' } });
    browserWindow.dispatch('keydown', textInput);

    expect(commands).toEqual(['right', 'previous']);
    expect(arrow.wasPrevented()).toBe(true);
    expect(ctrlQ.wasPrevented()).toBe(false);
    expect(textInput.wasPrevented()).toBe(false);
    stop();
  });

  it('repeats directions after a delay while keeping keyboard edge commands single-shot', () => {
    const commands: BigPictureCommand[] = [];
    const stop = startBigPictureInput({ onCommand: (command) => commands.push(command), isEnabled: () => true });
    const right = keyEvent('ArrowRight');
    browserWindow.dispatch('keydown', right);
    browserWindow.dispatch('keydown', { ...right, repeat: true });

    expect(commands).toEqual(['right']);
    vi.advanceTimersByTime(BIG_PICTURE_DIRECTION_REPEAT_DELAY_MS - 1);
    expect(commands).toEqual(['right']);
    vi.advanceTimersByTime(1);
    expect(commands).toEqual(['right', 'right']);
    vi.advanceTimersByTime(BIG_PICTURE_DIRECTION_REPEAT_INTERVAL_MS);
    expect(commands).toEqual(['right', 'right', 'right']);

    browserWindow.dispatch('keyup', right);
    vi.advanceTimersByTime(BIG_PICTURE_DIRECTION_REPEAT_INTERVAL_MS * 2);
    expect(commands).toEqual(['right', 'right', 'right']);

    browserWindow.dispatch('keydown', { key: 'Enter', code: 'Enter' });
    browserWindow.dispatch('keydown', { key: 'Enter', code: 'Enter', repeat: true });
    expect(commands.at(-1)).toBe('confirm');
    expect(commands.filter((command) => command === 'confirm')).toHaveLength(1);
    stop();
  });

  it('requires a fresh press after blur and never replays a held confirm on return', () => {
    const commands: BigPictureCommand[] = [];
    const stop = startBigPictureInput({ onCommand: (command) => commands.push(command), isEnabled: () => true });
    browserWindow.dispatch('keydown', { key: 'Enter', code: 'Enter', repeat: false });
    browserWindow.dispatch('blur');
    browserDocument.hasFocus = () => false;
    browserDocument.visibilityState = 'hidden';
    browserDocument.hidden = true;
    browserDocument.dispatch('visibilitychange');
    browserDocument.hasFocus = () => true;
    browserDocument.visibilityState = 'visible';
    browserDocument.hidden = false;

    browserWindow.dispatch('keydown', { key: 'Enter', code: 'Enter', repeat: true });
    expect(commands).toEqual(['confirm']);
    browserWindow.dispatch('keyup', { key: 'Enter', code: 'Enter' });
    browserWindow.dispatch('keydown', { key: 'Enter', code: 'Enter', repeat: false });
    expect(commands).toEqual(['confirm', 'confirm']);
    stop();
  });

  it('requires a fresh press after the enabled gate closes while a key is held', () => {
    const commands: BigPictureCommand[] = [];
    let enabled = true;
    const stop = startBigPictureInput({ onCommand: (command) => commands.push(command), isEnabled: () => enabled });
    browserWindow.dispatch('keydown', { key: 'Enter', code: 'Enter', repeat: false });
    enabled = false;
    // A repeat while the gate is closed causes the controller to release all
    // logical keys even though the physical key remains held.
    browserWindow.dispatch('keydown', { key: 'Enter', code: 'Enter', repeat: true });
    enabled = true;
    browserWindow.dispatch('keydown', { key: 'Enter', code: 'Enter', repeat: true });
    expect(commands).toEqual(['confirm']);
    browserWindow.dispatch('keyup', { key: 'Enter', code: 'Enter' });
    browserWindow.dispatch('keydown', { key: 'Enter', code: 'Enter', repeat: false });
    expect(commands).toEqual(['confirm', 'confirm']);
    stop();
  });

  it('emits standard gamepad button edges, identifies devices, and ignores unsupported mappings', () => {
    const pad = gamepad('DualSense Wireless Controller');
    pads = [pad];
    const commands: BigPictureCommand[] = [];
    const devices: BigPictureDevice[] = [];
    const stop = startBigPictureInput({
      onCommand: (command) => commands.push(command),
      onDevice: (device) => devices.push(device),
      isEnabled: () => true,
    });

    pad.buttons[0].pressed = true;
    vi.advanceTimersByTime(16);
    vi.advanceTimersByTime(1000);
    expect(commands).toEqual(['confirm']);
    expect(devices).toEqual(['playstation']);

    pad.buttons[0].pressed = false;
    vi.advanceTimersByTime(16);
    pad.buttons[0].pressed = true;
    vi.advanceTimersByTime(16);
    expect(commands).toEqual(['confirm', 'confirm']);

    pad.mapping = 'vendor';
    pad.buttons[1].pressed = true;
    vi.advanceTimersByTime(16);
    expect(commands).toEqual(['confirm', 'confirm']);
    pads = [];
    browserWindow.dispatch('gamepaddisconnected', { gamepad: pad });
    expect(devices).toEqual(['playstation', 'keyboard']);
    stop();
  });

  it('holds a direction with gamepad repeat and suppresses held buttons across reconnect', () => {
    const pad = gamepad();
    pads = [pad];
    const commands: BigPictureCommand[] = [];
    const stop = startBigPictureInput({ onCommand: (command) => commands.push(command), isEnabled: () => true });

    pad.axes[0] = 1;
    vi.advanceTimersByTime(16);
    expect(commands).toEqual(['right']);
    vi.advanceTimersByTime(BIG_PICTURE_DIRECTION_REPEAT_DELAY_MS - 1);
    expect(commands).toEqual(['right']);
    vi.advanceTimersByTime(1);
    expect(commands.length).toBe(2);
    vi.advanceTimersByTime(BIG_PICTURE_DIRECTION_REPEAT_INTERVAL_MS);
    expect(commands.length).toBe(3);

    pad.axes[0] = 0;
    pads = [];
    browserWindow.dispatch('gamepaddisconnected', { gamepad: pad });
    const reconnected = gamepad();
    reconnected.buttons[0].pressed = true;
    pads = [reconnected];
    browserWindow.dispatch('gamepadconnected', { gamepad: reconnected });
    vi.advanceTimersByTime(16);
    expect(commands.filter((command) => command === 'confirm')).toHaveLength(0);

    reconnected.buttons[0].pressed = false;
    vi.advanceTimersByTime(16);
    reconnected.buttons[0].pressed = true;
    vi.advanceTimersByTime(16);
    expect(commands.filter((command) => command === 'confirm')).toHaveLength(1);
    stop();
  });

  it('prioritizes one gamepad edge over a simultaneous direction in one poll', () => {
    const pad = gamepad();
    pads = [pad];
    const commands: BigPictureCommand[] = [];
    const stop = startBigPictureInput({ onCommand: (command) => commands.push(command), isEnabled: () => true });

    pad.axes[0] = 1;
    pad.buttons[0].pressed = true;
    vi.advanceTimersByTime(16);
    expect(commands).toEqual(['confirm']);
    vi.advanceTimersByTime(16);
    expect(commands).toEqual(['confirm', 'right']);
    stop();
  });

  it('does not resume a held gamepad button after disabled input or an API error', () => {
    const pad = gamepad();
    pads = [pad];
    let enabled = true;
    const commands: BigPictureCommand[] = [];
    const stop = startBigPictureInput({ onCommand: (command) => commands.push(command), isEnabled: () => enabled });

    enabled = false;
    pad.buttons[0].pressed = true;
    vi.advanceTimersByTime(16);
    enabled = true;
    vi.advanceTimersByTime(16);
    expect(commands).toEqual([]);
    pad.buttons[0].pressed = false;
    vi.advanceTimersByTime(16);
    pad.buttons[0].pressed = true;
    vi.advanceTimersByTime(16);
    expect(commands).toEqual(['confirm']);

    pad.buttons[0].pressed = false;
    pad.axes[0] = 1;
    vi.advanceTimersByTime(16);
    expect(commands.at(-1)).toBe('right');
    const navigatorStub = navigator as Navigator & { getGamepads: () => readonly FakePad[] };
    navigatorStub.getGamepads = () => { throw new Error('temporary gamepad failure'); };
    vi.advanceTimersByTime(BIG_PICTURE_DIRECTION_REPEAT_DELAY_MS + BIG_PICTURE_DIRECTION_REPEAT_INTERVAL_MS);
    const beforeRecovery = commands.length;
    vi.advanceTimersByTime(BIG_PICTURE_DIRECTION_REPEAT_INTERVAL_MS * 2);
    expect(commands.length).toBe(beforeRecovery);
    stop();
  });

  it('polls rarely while the window is in the background and resumes on focus', () => {
    const pad = gamepad();
    pads = [pad];
    let polls = 0;
    vi.stubGlobal('navigator', { getGamepads: () => { polls += 1; return pads; } });
    const commands: BigPictureCommand[] = [];
    const stop = startBigPictureInput({ onCommand: (command) => commands.push(command), isEnabled: () => true });

    browserDocument.hasFocus = () => false;
    browserWindow.dispatch('blur');
    polls = 0;
    vi.advanceTimersByTime(1000);
    expect(polls).toBeLessThanOrEqual(2);

    pad.buttons[0].pressed = true;
    vi.advanceTimersByTime(1000);
    expect(commands).toEqual([]);

    pad.buttons[0].pressed = false;
    browserDocument.hasFocus = () => true;
    browserWindow.dispatch('focus');
    vi.advanceTimersByTime(16);
    pad.buttons[0].pressed = true;
    vi.advanceTimersByTime(16);
    expect(commands).toEqual(['confirm']);
    stop();
  });

  it('stops requesting animation frames while the window is in the background', () => {
    const frames: FrameRequestCallback[] = [];
    Object.assign(browserWindow, {
      requestAnimationFrame: (callback: FrameRequestCallback) => frames.push(callback),
      cancelAnimationFrame: () => {},
    });
    const stop = startBigPictureInput({ onCommand: () => {}, isEnabled: () => true });
    expect(frames).toHaveLength(1);

    browserDocument.hasFocus = () => false;
    frames.shift()?.(0);
    vi.advanceTimersByTime(2000);
    expect(frames).toHaveLength(0);

    browserDocument.hasFocus = () => true;
    browserWindow.dispatch('focus');
    expect(frames).toHaveLength(1);
    stop();
  });

  it('suppresses input while disabled and removes every callback on teardown', () => {
    const commands: BigPictureCommand[] = [];
    const devices: BigPictureDevice[] = [];
    let enabled = false;
    const stop = startBigPictureInput({
      onCommand: (command) => commands.push(command),
      onDevice: (device) => devices.push(device),
      isEnabled: () => enabled,
    });

    browserWindow.dispatch('keydown', { key: 'Enter', code: 'Enter' });
    enabled = true;
    browserWindow.dispatch('keydown', { key: 'Enter', code: 'Enter', repeat: true });
    expect(commands).toEqual([]);

    stop();
    browserWindow.dispatch('keydown', { key: 'Enter', code: 'Enter' });
    browserWindow.dispatch('keydown', { key: 'ArrowDown', code: 'ArrowDown' });
    vi.advanceTimersByTime(BIG_PICTURE_DIRECTION_REPEAT_DELAY_MS + BIG_PICTURE_DIRECTION_REPEAT_INTERVAL_MS);
    expect(commands).toEqual([]);
    expect(devices).toEqual([]);
  });
});
