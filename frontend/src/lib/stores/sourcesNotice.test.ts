import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import type { Settings } from '../services/settings';

globalThis.document = {
  documentElement: {
    style: { setProperty: () => {} },
    classList: { toggle: () => {} },
  },
} as unknown as Document;

vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('../services/settings', () => ({
  getSettings: vi.fn(),
  saveSettings: vi.fn(),
  setupLibrary: vi.fn(),
  proposeLibraryPath: vi.fn(),
}));

const { getSettings, saveSettings } = await import('../services/settings');
const { settings, initSettings } = await import('./settings');
const { needsSourcesNotice, acceptSourcesNotice } = await import('./sourcesNotice');

function makeSettings(accepted: boolean): Settings {
  return { uiScale: 1, animationsEnabled: true, sourcesNoticeAccepted: accepted } as Settings;
}

async function load(accepted: boolean) {
  vi.mocked(getSettings).mockResolvedValue(makeSettings(accepted));
  await initSettings();
}

describe('sourcesNotice', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    settings.set(null);
  });

  it('требует уведомление, пока настройки не загружены', () => {
    expect(needsSourcesNotice()).toBe(true);
  });

  it('требует уведомление, пока оно не подтверждено', async () => {
    await load(false);
    expect(needsSourcesNotice()).toBe(true);
  });

  it('не требует уведомление повторно после подтверждения', async () => {
    await load(true);
    expect(needsSourcesNotice()).toBe(false);
  });

  it('сохраняет подтверждение и сообщает об успехе', async () => {
    await load(false);
    vi.mocked(saveSettings).mockResolvedValue(undefined as never);

    await expect(acceptSourcesNotice()).resolves.toBe(true);

    expect(saveSettings).toHaveBeenCalledTimes(1);
    expect(vi.mocked(saveSettings).mock.calls[0][0].sourcesNoticeAccepted).toBe(true);
    expect(get(settings)?.sourcesNoticeAccepted).toBe(true);
    expect(needsSourcesNotice()).toBe(false);
  });

  it('не засчитывает подтверждение, если сохранение упало', async () => {
    await load(false);
    vi.mocked(saveSettings).mockRejectedValue(new Error('disk full'));

    await expect(acceptSourcesNotice()).resolves.toBe(false);

    expect(get(settings)?.sourcesNoticeAccepted).toBe(false);
    expect(needsSourcesNotice()).toBe(true);
  });

  it('не засчитывает подтверждение, когда настройки ещё не загружены', async () => {
    await expect(acceptSourcesNotice()).resolves.toBe(false);

    expect(saveSettings).not.toHaveBeenCalled();
    expect(needsSourcesNotice()).toBe(true);
  });
});
