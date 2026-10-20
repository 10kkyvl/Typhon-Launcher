import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));

const { toast } = await import('./toasts');
const { reportGameModeError, reportGameModeStatus, resetGameModeNotices, DEDUPE_MS } = await import('./gamemode');
const { ru } = await import('../i18n/catalog/ru');

describe('reportGameModeError', () => {
  beforeEach(() => {
    vi.mocked(toast).mockClear();
    resetGameModeNotices();
  });

  it('shows the localized message for the code, not the technical detail', () => {
    reportGameModeError({ toggle: 'hide', code: 'gamemode.hide_needs_tray', detail: 'tray disabled' });
    expect(toast).toHaveBeenCalledWith(ru['gamemode.hide_needs_tray'], 'danger');
  });

  it('shows the general message for an unknown code', () => {
    reportGameModeError({ toggle: 'power', code: 'nope', detail: 'x' });
    expect(toast).toHaveBeenCalledWith(ru['gamemode.failed'], 'danger');
  });

  it('ignores an empty payload', () => {
    reportGameModeError(null);
    expect(toast).not.toHaveBeenCalled();
  });
});

describe('reportGameModeError dedupe', () => {
  beforeEach(() => {
    vi.mocked(toast).mockClear();
    resetGameModeNotices();
  });

  const info = { gameId: 'a', toggle: 'power', code: 'gamemode.power_failed', detail: 'x' };

  it('shows one toast for the same game and code inside the window', () => {
    reportGameModeError(info, 1000);
    reportGameModeError(info, 1000 + DEDUPE_MS - 1);
    expect(toast).toHaveBeenCalledTimes(1);
  });

  it('shows it again after the window', () => {
    reportGameModeError(info, 1000);
    reportGameModeError(info, 1000 + DEDUPE_MS);
    expect(toast).toHaveBeenCalledTimes(2);
  });

  it('keeps other games and other codes apart', () => {
    reportGameModeError(info, 1000);
    reportGameModeError({ ...info, gameId: 'b' }, 1001);
    reportGameModeError({ ...info, code: 'gamemode.priority_failed' }, 1002);
    expect(toast).toHaveBeenCalledTimes(3);
  });
});

describe('reportGameModeStatus', () => {
  beforeEach(() => {
    vi.mocked(toast).mockClear();
    resetGameModeNotices();
  });

  it('shows the localized toast once when the service is blocked', async () => {
    const load = async () => ({ blocked: true, code: 'gamemode.marker_corrupt' });
    await reportGameModeStatus(load, 1000);
    await reportGameModeStatus(load, 1001);
    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast).toHaveBeenCalledWith(ru['gamemode.marker_corrupt'], 'danger');
  });

  it('stays quiet when nothing is blocked', async () => {
    await reportGameModeStatus(async () => ({ blocked: false }));
    expect(toast).not.toHaveBeenCalled();
  });

  it('reports a failed status call instead of swallowing it', async () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
    await reportGameModeStatus(async () => {
      throw new Error('boom');
    });
    expect(toast).toHaveBeenCalledWith(ru['gamemode.failed'], 'danger');
    spy.mockRestore();
  });
});
