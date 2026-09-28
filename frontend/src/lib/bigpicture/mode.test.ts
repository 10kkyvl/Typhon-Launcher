import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

const testState = vi.hoisted(() => ({
  inWails: true,
  useWindowEvents: false,
  nativeFullscreen: false,
  listeners: new Map<string, () => void>(),
}));
const fakeWindow = vi.hoisted(() => ({
  Focus: vi.fn(),
  Fullscreen: vi.fn(),
  IsFullscreen: vi.fn(),
  IsMaximised: vi.fn(),
  Maximise: vi.fn(),
  Show: vi.fn(),
  UnFullscreen: vi.fn(),
  UnMaximise: vi.fn(),
  UnMinimise: vi.fn(),
}));
const fakeEvents = vi.hoisted(() => ({
  Once: vi.fn((eventName: string, callback: () => void) => {
    if (!testState.useWindowEvents) throw new Error('window events disabled');
    testState.listeners.set(eventName, callback);
    return () => testState.listeners.delete(eventName);
  }),
}));

vi.mock('@wailsio/runtime', () => ({ Events: fakeEvents, Window: fakeWindow }));
vi.mock('../services/backend', () => ({
  get inWails() {
    return testState.inWails;
  },
}));

function resetWindow() {
  for (const method of Object.values(fakeWindow)) method.mockReset();
  fakeEvents.Once.mockClear();
  testState.useWindowEvents = false;
  testState.nativeFullscreen = false;
  testState.listeners.clear();
  fakeWindow.Focus.mockResolvedValue(undefined);
  fakeWindow.Fullscreen.mockImplementation(async () => {
    testState.nativeFullscreen = true;
  });
  fakeWindow.IsFullscreen.mockImplementation(async () => testState.nativeFullscreen);
  fakeWindow.IsMaximised.mockResolvedValue(false);
  fakeWindow.Maximise.mockResolvedValue(undefined);
  fakeWindow.Show.mockResolvedValue(undefined);
  fakeWindow.UnFullscreen.mockImplementation(async () => {
    testState.nativeFullscreen = false;
  });
  fakeWindow.UnMaximise.mockResolvedValue(undefined);
  fakeWindow.UnMinimise.mockResolvedValue(undefined);
}

function emitWindowEvent(eventName: string) {
  testState.listeners.get(eventName)?.();
}

async function loadMode() {
  vi.resetModules();
  return import('./mode');
}

beforeEach(() => {
  testState.inWails = true;
  resetWindow();
});

describe('Big Picture mode lifecycle', () => {
  it('provides a browser preview without calling native window APIs', async () => {
    testState.inWails = false;
    const { bigPictureActive, enterBigPicture, exitBigPicture, restoreBigPictureWindow } = await loadMode();

    expect(get(bigPictureActive)).toBe(false);
    await enterBigPicture();
    expect(get(bigPictureActive)).toBe(true);

    await restoreBigPictureWindow();
    await exitBigPicture();

    expect(get(bigPictureActive)).toBe(false);
    expect(fakeWindow.IsFullscreen).not.toHaveBeenCalled();
    expect(fakeWindow.Fullscreen).not.toHaveBeenCalled();
    expect(fakeWindow.Show).not.toHaveBeenCalled();
  });

  it('does not advertise mode when native entry fails and can retry', async () => {
    const error = new Error('fullscreen unavailable');
    fakeWindow.Fullscreen.mockRejectedValueOnce(error);
    const { bigPictureActive, enterBigPicture } = await loadMode();

    await expect(enterBigPicture()).rejects.toBe(error);
    expect(get(bigPictureActive)).toBe(false);

    await enterBigPicture();
    expect(get(bigPictureActive)).toBe(true);
    expect(fakeWindow.Fullscreen).toHaveBeenCalledTimes(2);
  });

  it('waits for the native fullscreen completion event before activating', async () => {
    testState.useWindowEvents = true;
    const { bigPictureActive, enterBigPicture } = await loadMode();

    const entering = enterBigPicture();
    await vi.waitFor(() => expect(fakeEvents.Once).toHaveBeenCalled());
    expect(get(bigPictureActive)).toBe(false);

    emitWindowEvent('common:WindowFullscreen');
    await entering;

    expect(get(bigPictureActive)).toBe(true);
    expect(testState.listeners.size).toBe(0);
  });

  it('keeps mode active when exit fails and allows a retry', async () => {
    const error = new Error('fullscreen restore unavailable');
    const { bigPictureActive, enterBigPicture, exitBigPicture } = await loadMode();
    await enterBigPicture();

    fakeWindow.UnFullscreen.mockRejectedValueOnce(error);
    await expect(exitBigPicture()).rejects.toBe(error);
    expect(get(bigPictureActive)).toBe(true);

    await exitBigPicture();
    expect(get(bigPictureActive)).toBe(false);
    expect(fakeWindow.UnFullscreen).toHaveBeenCalledTimes(2);
  });

  it('does not toggle fullscreen again when a post-exit repair fails', async () => {
    testState.useWindowEvents = true;
    fakeWindow.Fullscreen.mockImplementationOnce(async () => {
      testState.nativeFullscreen = true;
      emitWindowEvent('common:WindowFullscreen');
    });
    const { bigPictureActive, enterBigPicture, exitBigPicture } = await loadMode();
    await enterBigPicture();

    const repairError = new Error('maximise state unavailable');
    fakeWindow.UnFullscreen.mockImplementationOnce(async () => {
      testState.nativeFullscreen = false;
      emitWindowEvent('common:WindowUnFullscreen');
    });
    fakeWindow.IsMaximised.mockReset();
    fakeWindow.IsMaximised.mockRejectedValueOnce(repairError).mockResolvedValue(false);

    await expect(exitBigPicture()).rejects.toBe(repairError);
    expect(get(bigPictureActive)).toBe(true);

    await exitBigPicture();

    expect(fakeWindow.UnFullscreen).toHaveBeenCalledTimes(1);
    expect(get(bigPictureActive)).toBe(false);
  });

  it('leaves a pre-existing fullscreen window untouched on exit', async () => {
    fakeWindow.IsFullscreen.mockResolvedValue(true);
    fakeWindow.IsMaximised.mockResolvedValue(true);
    const { bigPictureActive, enterBigPicture, exitBigPicture } = await loadMode();

    await enterBigPicture();
    expect(fakeWindow.Fullscreen).not.toHaveBeenCalled();
    expect(get(bigPictureActive)).toBe(true);

    await exitBigPicture();
    expect(fakeWindow.UnFullscreen).not.toHaveBeenCalled();
    expect(fakeWindow.Maximise).not.toHaveBeenCalled();
    expect(get(bigPictureActive)).toBe(false);
  });

  it('repairs a maximised state if the runtime loses it while leaving fullscreen', async () => {
    fakeWindow.IsMaximised.mockResolvedValueOnce(true).mockResolvedValueOnce(false);
    const { enterBigPicture, exitBigPicture } = await loadMode();

    await enterBigPicture();
    await exitBigPicture();

    expect(fakeWindow.Maximise).toHaveBeenCalledTimes(1);
  });

  it('serialises an exit queued while entry is still pending', async () => {
    testState.useWindowEvents = true;
    let finishFullscreen!: () => void;
    fakeWindow.Fullscreen.mockImplementationOnce(
      () => new Promise<void>((resolve) => {
        finishFullscreen = resolve;
      }),
    );
    const { bigPictureActive, enterBigPicture, exitBigPicture } = await loadMode();

    const entering = enterBigPicture();
    const exiting = exitBigPicture();
    await vi.waitFor(() => expect(finishFullscreen).toBeTypeOf('function'));
    expect(fakeWindow.UnFullscreen).not.toHaveBeenCalled();

    finishFullscreen();
    testState.nativeFullscreen = true;
    emitWindowEvent('common:WindowFullscreen');
    await entering;
    await vi.waitFor(() => expect(fakeWindow.UnFullscreen).toHaveBeenCalledTimes(1));
    expect(get(bigPictureActive)).toBe(true);

    emitWindowEvent('common:WindowUnFullscreen');
    await exiting;

    expect(fakeWindow.UnFullscreen).toHaveBeenCalledTimes(1);
    expect(get(bigPictureActive)).toBe(false);
  });

  it('accepts a matching native state when the completion event is absent', async () => {
    testState.useWindowEvents = true;
    vi.useFakeTimers();
    try {
      const { bigPictureActive, enterBigPicture } = await loadMode();
      const entering = enterBigPicture();
      for (let attempt = 0; attempt < 5 && testState.listeners.size === 0; attempt += 1) {
        await Promise.resolve();
      }
      expect(testState.listeners.size).toBe(1);

      vi.advanceTimersByTime(1500);
      await entering;

      expect(get(bigPictureActive)).toBe(true);
      expect(testState.listeners.size).toBe(0);
    } finally {
      vi.useRealTimers();
    }
  });

  it('keeps entry inactive when the bounded fallback sees the wrong state', async () => {
    testState.useWindowEvents = true;
    fakeWindow.Fullscreen.mockImplementationOnce(async () => undefined);
    vi.useFakeTimers();
    try {
      const { bigPictureActive, enterBigPicture } = await loadMode();
      const firstAttempt = enterBigPicture();
      for (let attempt = 0; attempt < 5 && testState.listeners.size === 0; attempt += 1) {
        await Promise.resolve();
      }
      vi.advanceTimersByTime(1500);
      await expect(firstAttempt).rejects.toThrow('native fullscreen transition');
      expect(get(bigPictureActive)).toBe(false);

      fakeWindow.Fullscreen.mockImplementationOnce(async () => {
        testState.nativeFullscreen = true;
      });
      const retry = enterBigPicture();
      for (let attempt = 0; attempt < 5 && testState.listeners.size === 0; attempt += 1) {
        await Promise.resolve();
      }
      vi.advanceTimersByTime(1500);
      await retry;

      expect(get(bigPictureActive)).toBe(true);
    } finally {
      vi.useRealTimers();
    }
  });

  it('keeps the active snapshot when exit fallback sees fullscreen still active', async () => {
    testState.useWindowEvents = true;
    fakeWindow.Fullscreen.mockImplementationOnce(async () => {
      testState.nativeFullscreen = true;
      emitWindowEvent('common:WindowFullscreen');
    });
    const { bigPictureActive, enterBigPicture, exitBigPicture } = await loadMode();
    await enterBigPicture();

    fakeWindow.UnFullscreen.mockImplementationOnce(async () => undefined);
    vi.useFakeTimers();
    try {
      const firstExit = exitBigPicture();
      for (let attempt = 0; attempt < 5 && testState.listeners.size === 0; attempt += 1) {
        await Promise.resolve();
      }
      vi.advanceTimersByTime(1500);
      await expect(firstExit).rejects.toThrow('native fullscreen transition');
      expect(get(bigPictureActive)).toBe(true);

      fakeWindow.UnFullscreen.mockImplementationOnce(async () => {
        testState.nativeFullscreen = false;
      });
      const retry = exitBigPicture();
      for (let attempt = 0; attempt < 5 && testState.listeners.size === 0; attempt += 1) {
        await Promise.resolve();
      }
      vi.advanceTimersByTime(1500);
      await retry;

      expect(get(bigPictureActive)).toBe(false);
      expect(fakeWindow.UnFullscreen).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });

  it('restores an active window in order and only while active', async () => {
    const inactive = await loadMode();
    await inactive.restoreBigPictureWindow();
    expect(fakeWindow.Show).not.toHaveBeenCalled();

    const { enterBigPicture, restoreBigPictureWindow } = await loadMode();
    await enterBigPicture();
    await restoreBigPictureWindow();

    expect(fakeWindow.Show).toHaveBeenCalledTimes(1);
    expect(fakeWindow.UnMinimise).toHaveBeenCalledTimes(1);
    expect(fakeWindow.Focus).toHaveBeenCalledTimes(1);
    expect(
      fakeWindow.Show.mock.invocationCallOrder[0],
    ).toBeLessThan(fakeWindow.UnMinimise.mock.invocationCallOrder[0]);
    expect(
      fakeWindow.UnMinimise.mock.invocationCallOrder[0],
    ).toBeLessThan(fakeWindow.Focus.mock.invocationCallOrder[0]);
  });

  it('abandons a pending restore when exit is requested', async () => {
    let finishShow!: () => void;
    fakeWindow.Show.mockImplementationOnce(
      () => new Promise<void>((resolve) => {
        finishShow = resolve;
      }),
    );
    const { enterBigPicture, exitBigPicture, restoreBigPictureWindow } = await loadMode();
    await enterBigPicture();

    const restoring = restoreBigPictureWindow();
    await vi.waitFor(() => expect(finishShow).toBeTypeOf('function'));
    const exiting = exitBigPicture();
    finishShow();

    await restoring;
    await exiting;
    expect(fakeWindow.UnMinimise).not.toHaveBeenCalled();
    expect(fakeWindow.Focus).not.toHaveBeenCalled();
  });
});
