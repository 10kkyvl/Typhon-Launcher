export type BigPictureCommand =
  | 'up'
  | 'down'
  | 'left'
  | 'right'
  | 'confirm'
  | 'back'
  | 'menu'
  | 'previous'
  | 'next';

export type BigPictureDevice = 'keyboard' | 'xbox' | 'playstation' | 'generic';

export interface StartBigPictureInputOptions {
  onCommand: (command: BigPictureCommand) => void;
  isEnabled: () => boolean;
  onDevice?: (device: BigPictureDevice) => void;
}

const DEADZONE = 0.25;
const DIRECTION_REPEAT_DELAY_MS = 300;
const DIRECTION_REPEAT_INTERVAL_MS = 100;

type DirectionCommand = Extract<BigPictureCommand, 'up' | 'down' | 'left' | 'right'>;

interface KeyboardEventLike {
  key?: string;
  code?: string;
  repeat?: boolean;
  isComposing?: boolean;
  ctrlKey?: boolean;
  metaKey?: boolean;
  altKey?: boolean;
  target?: EventTarget | null;
  preventDefault?: () => void;
}

interface GamepadButtonLike {
  pressed?: boolean;
  value?: number;
}

interface GamepadLike {
  id?: string;
  index?: number;
  mapping?: string;
  connected?: boolean;
  axes?: readonly number[];
  buttons?: readonly (GamepadButtonLike | null | undefined)[];
}

interface GamepadEventLike extends Event {
  gamepad?: GamepadLike;
}

interface GamepadState {
  key: string;
  identity: string;
  blockedUntilNeutral: boolean;
  connected: boolean;
  buttons: Map<number, boolean>;
  direction: DirectionCommand | null;
  device: BigPictureDevice;
}

interface TimerHandle {
  cancel: () => void;
}

interface RepeatState {
  command: DirectionCommand;
  timer: TimerHandle | null;
}

const DIRECTION_BY_KEY: Readonly<Record<string, DirectionCommand>> = {
  ArrowUp: 'up',
  ArrowDown: 'down',
  ArrowLeft: 'left',
  ArrowRight: 'right',
};

const COMMAND_BY_KEY: Readonly<Record<string, BigPictureCommand>> = {
  Enter: 'confirm',
  ' ': 'confirm',
  Space: 'confirm',
  Spacebar: 'confirm',
  Escape: 'back',
  F10: 'menu',
  q: 'previous',
  e: 'next',
};

const GAMEPAD_COMMAND_BUTTONS: Readonly<Record<number, BigPictureCommand>> = {
  0: 'confirm', // A / Cross
  1: 'back', // B / Circle
  4: 'previous', // LB / L1
  5: 'next', // RB / R1
  9: 'menu', // Start / Options
};

const GAMEPAD_DIRECTION_BUTTONS: Readonly<Record<number, DirectionCommand>> = {
  12: 'up',
  13: 'down',
  14: 'left',
  15: 'right',
};

function getDocumentSafe(): (Document & { hasFocus?: () => boolean }) | undefined {
  return typeof document !== 'undefined' ? (document as Document & { hasFocus?: () => boolean }) : undefined;
}

function getWindowSafe(): (Window & {
  requestAnimationFrame?: (callback: FrameRequestCallback) => number;
  cancelAnimationFrame?: (handle: number) => void;
}) | undefined {
  return typeof window !== 'undefined'
    ? (window as Window & {
        requestAnimationFrame?: (callback: FrameRequestCallback) => number;
        cancelAnimationFrame?: (handle: number) => void;
      })
    : undefined;
}

function getNavigatorSafe(): (Navigator & { getGamepads?: () => readonly (GamepadLike | null)[] }) | undefined {
  return typeof navigator !== 'undefined'
    ? (navigator as Navigator & { getGamepads?: () => readonly (GamepadLike | null)[] })
    : undefined;
}

function getAnimationFrameSafe(): {
  request: (callback: FrameRequestCallback) => number;
  cancel?: (handle: number) => void;
} | null {
  const currentWindow = getWindowSafe();
  if (currentWindow?.requestAnimationFrame) {
    return {
      request: currentWindow.requestAnimationFrame.bind(currentWindow),
      cancel: currentWindow.cancelAnimationFrame?.bind(currentWindow),
    };
  }

  const runtime = globalThis as typeof globalThis & {
    requestAnimationFrame?: (callback: FrameRequestCallback) => number;
    cancelAnimationFrame?: (handle: number) => void;
  };
  if (runtime.requestAnimationFrame) {
    return { request: runtime.requestAnimationFrame, cancel: runtime.cancelAnimationFrame };
  }
  return null;
}

function documentIsActive(): boolean {
  const currentDocument = getDocumentSafe();
  if (!currentDocument) return true;

  if (currentDocument.hidden === true || currentDocument.visibilityState === 'hidden') return false;
  try {
    if (typeof currentDocument.hasFocus === 'function' && !currentDocument.hasFocus()) return false;
  } catch {
    // A minimal document test double may not implement hasFocus correctly.
  }
  return true;
}

function targetIsTextInput(target: EventTarget | null | undefined): boolean {
  let node = target as (EventTarget & {
    tagName?: string;
    isContentEditable?: boolean;
    parentElement?: unknown;
    getAttribute?: (name: string) => string | null;
  }) | null | undefined;
  let depth = 0;

  while (node && depth < 32) {
    const tagName = typeof node.tagName === 'string' ? node.tagName.toLowerCase() : '';
    if (tagName === 'input' || tagName === 'textarea' || tagName === 'select') return true;
    if (node.isContentEditable === true) return true;

    try {
      const contentEditable = node.getAttribute?.('contenteditable');
      if (contentEditable !== null && contentEditable !== 'false') return true;
    } catch {
      // Ignore incomplete test doubles.
    }

    node = (node.parentElement as typeof node | null | undefined) ?? null;
    depth += 1;
  }

  return false;
}

function normalizeKeyboardKey(event: KeyboardEventLike): string {
  // `key` is localized (for example, Й/У on a Russian layout), while the
  // physical codes keep the Q/E shelf shortcuts stable.
  if (event.code === 'KeyQ') return 'q';
  if (event.code === 'KeyE') return 'e';
  if (event.key) return event.key.length === 1 ? event.key.toLowerCase() : event.key;
  return event.code ?? '';
}

function buttonPressed(button: GamepadButtonLike | null | undefined): boolean {
  if (!button) return false;
  return button.pressed === true || (typeof button.value === 'number' && button.value > 0.5);
}

function axisValue(axes: readonly number[] | undefined, index: number): number {
  const value = axes?.[index];
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function directionFromAxis(axes: readonly number[] | undefined): DirectionCommand | null {
  const horizontal = axisValue(axes, 0);
  const vertical = axisValue(axes, 1);
  const horizontalMagnitude = Math.abs(horizontal);
  const verticalMagnitude = Math.abs(vertical);

  if (horizontalMagnitude <= DEADZONE && verticalMagnitude <= DEADZONE) return null;
  if (horizontalMagnitude >= verticalMagnitude) return horizontal > 0 ? 'right' : 'left';
  return vertical > 0 ? 'down' : 'up';
}

function gamepadIdentity(pad: GamepadLike, index: number): string {
  const id = typeof pad.id === 'string' ? pad.id : '';
  return `${index}:${id}`;
}

function classifyGamepad(pad: GamepadLike): BigPictureDevice {
  const id = typeof pad.id === 'string' ? pad.id.toLowerCase() : '';
  if (/(?:x[-\s]?box|xinput|microsoft[\s_-]*(?:xbox|controller|gamepad))/.test(id)) return 'xbox';
  if (/(?:playstation|sony[\s_-]*(?:interactive[\s_-]*)?entertainment|dualshock|dualsense|ps[345](?:\b|[-_ ]))/.test(id)) {
    return 'playstation';
  }
  return 'generic';
}

function makeTimer(callback: () => void, delay: number, repeat: boolean): TimerHandle {
  if (repeat) {
    const handle = setInterval(callback, delay);
    return { cancel: () => clearInterval(handle) };
  }
  const handle = setTimeout(callback, delay);
  return { cancel: () => clearTimeout(handle) };
}

function addListener(target: EventTarget | undefined, type: string, listener: EventListener): () => void {
  if (!target || typeof target.addEventListener !== 'function') return () => undefined;
  target.addEventListener(type, listener);
  return () => target.removeEventListener(type, listener);
}

/**
 * Start Big Picture keyboard and standard Gamepad input.
 *
 * Keyboard mapping: arrows navigate; Enter/Space confirms; Escape goes back;
 * F10 opens the menu; Q/E move to the previous/next shelf item. Standard
 * Gamepad mapping uses D-pad 12-15 or the left stick for directions, A/Cross
 * 0 for confirm, B/Circle 1 for back, Start/Options 9 for menu, and LB/L1
 * 4 plus RB/R1 5 for previous/next. Unsupported gamepad mappings are ignored.
 *
 * The return value tears down every listener, timer, and polling callback.
 */
export function startBigPictureInput(options: StartBigPictureInputOptions): () => void {
  const { onCommand, isEnabled, onDevice } = options;
  let disposed = false;
  let lastDevice: BigPictureDevice | null = null;
  let observedGamepad = false;

  const keyboardKeys = new Set<string>();
  const keyboardRepeats = new Map<string, RepeatState>();
  const gamepads = new Map<string, GamepadState>();
  const removeListeners: Array<() => void> = [];
  let cancelPoll: (() => void) | null = null;

  function canActivate(): boolean {
    if (disposed || !documentIsActive()) return false;
    try {
      return isEnabled();
    } catch {
      return false;
    }
  }

  function reportDevice(device: BigPictureDevice): void {
    if (lastDevice === device || !onDevice || disposed) return;
    lastDevice = device;
    onDevice(device);
  }

  function emit(command: BigPictureCommand, device: BigPictureDevice): void {
    if (!canActivate()) return;
    reportDevice(device);
    onCommand(command);
  }

  function cancelRepeat(repeat: RepeatState | undefined): void {
    repeat?.timer?.cancel();
    if (repeat) repeat.timer = null;
  }

  function stopKeyboardRepeat(key: string): void {
    const repeat = keyboardRepeats.get(key);
    cancelRepeat(repeat);
    keyboardRepeats.delete(key);
  }

  function startKeyboardRepeat(key: string, command: DirectionCommand): void {
    stopKeyboardRepeat(key);
    const repeat: RepeatState = { command, timer: null };
    const delayed = makeTimer(() => {
      if (disposed || !keyboardKeys.has(key) || !canActivate()) {
        stopKeyboardRepeat(key);
        return;
      }
      emit(command, 'keyboard');
      if (disposed || !keyboardKeys.has(key) || keyboardRepeats.get(key) !== repeat || !canActivate()) {
        stopKeyboardRepeat(key);
        return;
      }
      repeat.timer = makeTimer(() => {
        if (disposed || !keyboardKeys.has(key) || !canActivate()) {
          stopKeyboardRepeat(key);
          return;
        }
        emit(command, 'keyboard');
      }, DIRECTION_REPEAT_INTERVAL_MS, true);
    }, DIRECTION_REPEAT_DELAY_MS, false);
    repeat.timer = delayed;
    keyboardRepeats.set(key, repeat);
  }

  function stopAllRepeats(): void {
    for (const repeat of keyboardRepeats.values()) cancelRepeat(repeat);
    keyboardRepeats.clear();
    for (const state of gamepads.values()) {
      state.direction = null;
    }
  }

  function suspendInputs(): void {
    keyboardKeys.clear();
    stopAllRepeats();
    for (const state of gamepads.values()) {
      state.blockedUntilNeutral = true;
      state.connected = false;
    }
  }

  function onKeyDown(rawEvent: Event): void {
    const event = rawEvent as KeyboardEventLike;
    const key = normalizeKeyboardKey(event);
    const direction = DIRECTION_BY_KEY[key];
    const command = direction ?? COMMAND_BY_KEY[key];
    if (
      !command ||
      event.isComposing ||
      event.ctrlKey === true ||
      event.metaKey === true ||
      event.altKey === true ||
      targetIsTextInput(event.target)
    ) return;

    if (!canActivate()) {
      suspendInputs();
      return;
    }

    // Browsers may deliver a repeat keydown after a WebView regains focus. A
    // key with no tracked initial press must not resurrect a held command.
    if (event.repeat && !keyboardKeys.has(key)) return;

    const alreadyPressed = keyboardKeys.has(key);
    keyboardKeys.add(key);
    event.preventDefault?.();
    if (alreadyPressed) return;

    if (direction) {
      emit(direction, 'keyboard');
      startKeyboardRepeat(key, direction);
    } else {
      // Keyboard confirm/back/menu/previous/next are edge-triggered. Holding
      // a key never generates another activation until keyup + keydown.
      emit(command, 'keyboard');
    }
  }

  function onKeyUp(rawEvent: Event): void {
    const event = rawEvent as KeyboardEventLike;
    const key = normalizeKeyboardKey(event);
    if (!keyboardKeys.has(key)) return;
    keyboardKeys.delete(key);
    stopKeyboardRepeat(key);
  }

  function readButtons(pad: GamepadLike): Map<number, boolean> {
    const result = new Map<number, boolean>();
    for (let index = 0; index < (pad.buttons?.length ?? 0); index += 1) {
      result.set(index, buttonPressed(pad.buttons?.[index]));
    }
    return result;
  }

  function controlsAreNeutral(pad: GamepadLike, buttons: ReadonlyMap<number, boolean>): boolean {
    for (const pressed of buttons.values()) {
      if (pressed) return false;
    }
    const axes = pad.axes ?? [];
    for (const value of axes) {
      if (typeof value === 'number' && Number.isFinite(value) && Math.abs(value) > DEADZONE) return false;
    }
    return true;
  }

  function setGamepadDirection(state: GamepadState, direction: DirectionCommand | null): void {
    if (state.direction === direction) return;
    state.direction = direction;
    const repeatKey = `gamepad:${state.key}`;
    const previous = keyboardRepeats.get(repeatKey);
    cancelRepeat(previous);
    keyboardRepeats.delete(repeatKey);
    if (!direction) return;

    emit(direction, state.device);
    const repeat: RepeatState = { command: direction, timer: null };
    repeat.timer = makeTimer(() => {
      const current = gamepads.get(state.key);
      if (disposed || !current || current !== state || current.direction !== direction || current.blockedUntilNeutral || !canActivate()) {
        cancelRepeat(repeat);
        keyboardRepeats.delete(repeatKey);
        return;
      }
      emit(direction, state.device);
      if (
        disposed ||
        !gamepads.has(state.key) ||
        gamepads.get(state.key) !== state ||
        state.direction !== direction ||
        state.blockedUntilNeutral ||
        keyboardRepeats.get(repeatKey) !== repeat ||
        !canActivate()
      ) {
        cancelRepeat(repeat);
        keyboardRepeats.delete(repeatKey);
        return;
      }
      repeat.timer = makeTimer(() => {
        const latest = gamepads.get(state.key);
        if (disposed || !latest || latest !== state || latest.direction !== direction || latest.blockedUntilNeutral || !canActivate()) {
          cancelRepeat(repeat);
          keyboardRepeats.delete(repeatKey);
          return;
        }
        emit(direction, state.device);
      }, DIRECTION_REPEAT_INTERVAL_MS, true);
    }, DIRECTION_REPEAT_DELAY_MS, false);
    keyboardRepeats.set(repeatKey, repeat);
  }

  function directionFromStandardPad(pad: GamepadLike, buttons: ReadonlyMap<number, boolean>): DirectionCommand | null {
    const buttonDirection = Object.entries(GAMEPAD_DIRECTION_BUTTONS).find(([index]) => buttons.get(Number(index)) === true)?.[1];
    return buttonDirection ?? directionFromAxis(pad.axes);
  }

  function updateGamepad(pad: GamepadLike, index: number, active: boolean): void {
    observedGamepad = true;
    const key = String(typeof pad.index === 'number' ? pad.index : index);
    const identity = gamepadIdentity(pad, Number(key));
    const existingState = gamepads.get(key);
    const wasConnected = existingState?.connected === true;
    let state = existingState;
    if (!state || state.identity !== identity) {
      if (state) setGamepadDirection(state, null);
      state = {
        key,
        identity,
        blockedUntilNeutral: true,
        connected: true,
        buttons: new Map(),
        direction: null,
        device: classifyGamepad(pad),
      };
      gamepads.set(key, state);
    }

    state.connected = true;
    state.device = classifyGamepad(pad);
    if (!wasConnected) reportDevice(state.device);
    const currentButtons = readButtons(pad);
    if (!active) {
      state.blockedUntilNeutral = true;
      state.buttons = currentButtons;
      setGamepadDirection(state, null);
      return;
    }

    if (state.blockedUntilNeutral) {
      state.buttons = currentButtons;
      setGamepadDirection(state, null);
      if (controlsAreNeutral(pad, currentButtons)) state.blockedUntilNeutral = false;
      return;
    }

    if (pad.mapping === 'standard') {
      let edgeCommand: BigPictureCommand | null = null;
      for (const [buttonIndex, command] of Object.entries(GAMEPAD_COMMAND_BUTTONS)) {
        const indexNumber = Number(buttonIndex);
        const pressed = currentButtons.get(indexNumber) === true;
        if (edgeCommand === null && pressed && state.buttons.get(indexNumber) !== true) edgeCommand = command;
      }

      // A single poll can contain a face-button edge and a stick direction.
      // Prefer the edge and defer the direction until the next poll so a
      // confirm cannot also move the focused card in the same frame.
      if (edgeCommand !== null) {
        emit(edgeCommand, state.device);
        setGamepadDirection(state, null);
      } else {
        setGamepadDirection(state, directionFromStandardPad(pad, currentButtons));
      }
    } else {
      // Browser gamepads without the standard mapping have no safe portable
      // button layout. Track neutral state for reconnect safety, but do not
      // guess at commands from vendor-specific indexes.
      setGamepadDirection(state, null);
    }
    state.buttons = currentButtons;
  }

  function markMissingGamepads(seen: ReadonlySet<string>): void {
    for (const [key, state] of gamepads) {
      if (seen.has(key)) continue;
      state.connected = false;
      state.blockedUntilNeutral = true;
      state.buttons.clear();
      setGamepadDirection(state, null);
    }
  }

  function pollGamepads(): void {
    if (disposed) return;
    const navigatorSafe = getNavigatorSafe();
    const getGamepads = navigatorSafe?.getGamepads;
    if (typeof getGamepads !== 'function') return;

    let pads: readonly (GamepadLike | null)[] = [];
    try {
      pads = getGamepads.call(navigatorSafe) ?? [];
    } catch {
      // A temporarily unavailable browser API must release any repeat loop;
      // otherwise a stale axis can keep moving the UI after disconnect.
      suspendInputs();
      return;
    }

    const active = canActivate();
    if (!active) suspendInputs();
    const seen = new Set<string>();
    pads.forEach((pad, index) => {
      if (!pad || pad.connected === false) return;
      const key = String(typeof pad.index === 'number' ? pad.index : index);
      seen.add(key);
      updateGamepad(pad, index, active);
    });
    markMissingGamepads(seen);
    if (observedGamepad && [...gamepads.values()].every((state) => !state.connected)) reportDevice('keyboard');
  }

  function onGamepadConnected(rawEvent: Event): void {
    const event = rawEvent as GamepadEventLike;
    const pad = event.gamepad;
    if (!pad) return;
    // The browser's connected event itself is only a hint. Polling reads the
    // current state and starts with blockedUntilNeutral to avoid replaying a
    // button held before the reconnect.
    pollGamepads();
  }

  function onGamepadDisconnected(rawEvent: Event): void {
    const event = rawEvent as GamepadEventLike;
    const pad = event.gamepad;
    if (!pad) return;
    const key = String(typeof pad.index === 'number' ? pad.index : 0);
    const state = gamepads.get(key);
    if (state) {
      state.connected = false;
      state.blockedUntilNeutral = true;
      state.buttons.clear();
      setGamepadDirection(state, null);
    }
    if (observedGamepad && [...gamepads.values()].every((candidate) => !candidate.connected)) reportDevice('keyboard');
  }

  function schedulePoll(): void {
    if (disposed || cancelPoll) return;
    const animationFrame = getAnimationFrameSafe();
    if (animationFrame) {
      let cancelled = false;
      const handle = animationFrame.request(() => {
        if (cancelled) return;
        cancelPoll = null;
        pollGamepads();
        schedulePoll();
      });
      cancelPoll = () => {
        cancelled = true;
        animationFrame.cancel?.(handle);
        cancelPoll = null;
      };
      return;
    }

    let cancelled = false;
    const handle = setTimeout(() => {
      if (cancelled) return;
      cancelPoll = null;
      pollGamepads();
      schedulePoll();
    }, 16);
    cancelPoll = () => {
      cancelled = true;
      clearTimeout(handle);
      cancelPoll = null;
    };
  }

  const eventTarget = (getWindowSafe() ?? getDocumentSafe()) as EventTarget | undefined;
  removeListeners.push(addListener(eventTarget, 'keydown', onKeyDown as EventListener));
  removeListeners.push(addListener(eventTarget, 'keyup', onKeyUp as EventListener));
  removeListeners.push(addListener(eventTarget, 'blur', () => suspendInputs()));
  // Some WebViews restore the native window without delivering blur to the
  // page. Treat a subsequent focus as a fresh input session as well, so a
  // key held during the transition cannot replay a confirm command.
  removeListeners.push(addListener(eventTarget, 'focus', () => suspendInputs()));
  removeListeners.push(addListener(eventTarget, 'gamepadconnected', onGamepadConnected));
  removeListeners.push(addListener(eventTarget, 'gamepaddisconnected', onGamepadDisconnected));

  const currentDocument = getDocumentSafe();
  removeListeners.push(addListener(currentDocument, 'visibilitychange', () => suspendInputs()));

  pollGamepads();
  schedulePoll();

  return () => {
    if (disposed) return;
    disposed = true;
    cancelPoll?.();
    cancelPoll = null;
    suspendInputs();
    for (const remove of removeListeners) remove();
    removeListeners.length = 0;
    gamepads.clear();
    lastDevice = null;
  };
}

// Exported for focused unit tests and to keep the production constants in one
// place. Consumers should normally use startBigPictureInput only.
export const BIG_PICTURE_INPUT_DEADZONE = DEADZONE;
export const BIG_PICTURE_DIRECTION_REPEAT_DELAY_MS = DIRECTION_REPEAT_DELAY_MS;
export const BIG_PICTURE_DIRECTION_REPEAT_INTERVAL_MS = DIRECTION_REPEAT_INTERVAL_MS;
