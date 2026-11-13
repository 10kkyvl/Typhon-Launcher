import { beforeEach, describe, expect, it, vi } from 'vitest';

const handlers: Array<() => void> = [];

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((_name: string, handler: () => void) => {
      handlers.push(handler);
      return () => {};
    }),
  },
}));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('./emulatorPrompt', () => ({ askForEmulator: vi.fn() }));
vi.mock('../services/library', () => ({ launchGame: vi.fn() }));
vi.mock('../../../bindings/typhon/pendinglaunchservice', () => ({ TakePending: vi.fn() }));

const { toast } = await import('./toasts');
const { askForEmulator } = await import('./emulatorPrompt');
const { launchGame } = await import('../services/library');
const { TakePending } = await import('../../../bindings/typhon/pendinglaunchservice');
const { takePendingLaunch, initPendingLaunch } = await import('./pendingLaunch');
const { ru } = await import('../i18n/catalog/ru');

const pending = { gameId: 'g1', platform: 'ps2', code: 'emulators.not_configured' };
const nothing = { gameId: '', platform: '', code: '' };
const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

describe('takePendingLaunch', () => {
  beforeEach(() => {
    handlers.length = 0;
    vi.mocked(toast).mockReset();
    vi.mocked(askForEmulator).mockReset();
    vi.mocked(launchGame).mockReset();
    vi.mocked(TakePending).mockReset();
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  it('asks for the emulator and launches the game again once it is set', async () => {
    vi.mocked(TakePending).mockResolvedValue(pending);
    vi.mocked(askForEmulator).mockResolvedValue(true);
    vi.mocked(launchGame).mockResolvedValue(true);
    await takePendingLaunch();
    expect(askForEmulator).toHaveBeenCalledWith('ps2', 'emulators.not_configured');
    expect(launchGame).toHaveBeenCalledWith('g1');
    expect(toast).not.toHaveBeenCalled();
  });

  it('does nothing when nothing is waiting', async () => {
    vi.mocked(TakePending).mockResolvedValue(nothing);
    await takePendingLaunch();
    expect(askForEmulator).not.toHaveBeenCalled();
    expect(launchGame).not.toHaveBeenCalled();
  });

  it('does not launch when the prompt is dismissed', async () => {
    vi.mocked(TakePending).mockResolvedValue(pending);
    vi.mocked(askForEmulator).mockResolvedValue(false);
    await takePendingLaunch();
    expect(launchGame).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
  });

  it('shows a toast when the repeated launch fails', async () => {
    vi.mocked(TakePending).mockResolvedValue(pending);
    vi.mocked(askForEmulator).mockResolvedValue(true);
    vi.mocked(launchGame).mockRejectedValue(new Error('boom'));
    await takePendingLaunch();
    expect(toast).toHaveBeenCalledWith(expect.any(String), 'danger');
  });

  it('shows a toast when the pending prompt cannot be read', async () => {
    vi.mocked(TakePending).mockRejectedValue(new Error('boom'));
    await takePendingLaunch();
    expect(askForEmulator).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith(ru['games.errorPlayFailed'], 'danger');
  });
});

describe('initPendingLaunch', () => {
  beforeEach(() => {
    handlers.length = 0;
    vi.mocked(toast).mockReset();
    vi.mocked(askForEmulator).mockReset();
    vi.mocked(launchGame).mockReset();
    vi.mocked(TakePending).mockReset();
  });

  it('picks up a prompt left before the page loaded (cold start)', async () => {
    vi.mocked(TakePending).mockResolvedValueOnce(pending).mockResolvedValue(nothing);
    vi.mocked(askForEmulator).mockResolvedValue(true);
    vi.mocked(launchGame).mockResolvedValue(true);
    initPendingLaunch();
    await flush();
    expect(askForEmulator).toHaveBeenCalledTimes(1);
    expect(launchGame).toHaveBeenCalledWith('g1');
  });

  it('reads the prompt on every event and handles it once', async () => {
    vi.mocked(TakePending).mockResolvedValue(nothing);
    vi.mocked(askForEmulator).mockResolvedValue(true);
    vi.mocked(launchGame).mockResolvedValue(true);
    initPendingLaunch();
    await flush();
    expect(handlers).toHaveLength(1);
    vi.mocked(TakePending).mockResolvedValueOnce(pending).mockResolvedValue(nothing);
    handlers[0]();
    await flush();
    handlers[0]();
    await flush();
    expect(TakePending).toHaveBeenCalledTimes(3);
    expect(askForEmulator).toHaveBeenCalledTimes(1);
    expect(launchGame).toHaveBeenCalledTimes(1);
  });
});
