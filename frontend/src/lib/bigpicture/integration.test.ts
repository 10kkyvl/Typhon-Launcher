import { afterEach, describe, expect, it, vi } from 'vitest';

const bridge = vi.hoisted(() => ({
  listeners: new Map<string, (event: { data: unknown }) => void>(),
  play: vi.fn<() => Promise<void>>(),
}));
vi.mock('@wailsio/runtime', () => ({
  Events: { On: (name: string, callback: (event: { data: unknown }) => void) => bridge.listeners.set(name, callback) },
}));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../../../bindings/typhon/internal/library', () => ({ Service: {
  GetGames: async () => [], GetRunningGames: async () => [], PlayGame: bridge.play,
} }));

import { initLibrary, runningGames } from '../stores/library';
import { launchGame } from '../services/library';
import { createBigPictureSession } from './session';

afterEach(() => { bridge.listeners.clear(); bridge.play.mockReset(); });

describe('Wails events → library store → Big Picture return', () => {
  it('restores the selected game once through the same event path as the app', async () => {
    const onReturn = vi.fn(async () => {});
    const session = createBigPictureSession({ launch: launchGame, onReturn, onReturnError: vi.fn() });
    await initLibrary();
    const unsubscribe = runningGames.subscribe(session.observe);
    try {
      bridge.play.mockImplementationOnce(async () => {
        bridge.listeners.get('game:started')?.({ data: { gameId: 'owned', sessionSeconds: 0 } });
      });
      expect(await session.launch('owned')).toBe(true);
      bridge.listeners.get('game:stopped')?.({ data: { gameId: 'other', sessionSeconds: 0 } });
      expect(onReturn).not.toHaveBeenCalled();
      bridge.listeners.get('game:stopped')?.({ data: { gameId: 'owned', sessionSeconds: 0 } });
      expect(onReturn).toHaveBeenCalledTimes(1);
      expect(onReturn).toHaveBeenCalledWith('owned');
    } finally { unsubscribe(); session.dispose(); }
  });

  it('does not arm return-to-launcher after a native launch cancellation', async () => {
    const onReturn = vi.fn(async () => {});
    const session = createBigPictureSession({ launch: launchGame, onReturn, onReturnError: vi.fn() });
    await initLibrary();
    const unsubscribe = runningGames.subscribe(session.observe);
    try {
      bridge.play.mockRejectedValueOnce(new Error('typhon:library.launch_cancelled: cancelled'));
      expect(await session.launch('cancelled')).toBe(false);
      bridge.listeners.get('game:started')?.({ data: { gameId: 'cancelled', sessionSeconds: 0 } });
      bridge.listeners.get('game:stopped')?.({ data: { gameId: 'cancelled', sessionSeconds: 0 } });
      expect(onReturn).not.toHaveBeenCalled();
    } finally { unsubscribe(); session.dispose(); }
  });
});
