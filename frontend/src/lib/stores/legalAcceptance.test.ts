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
vi.mock('../services/legal', () => ({ legalVersion: vi.fn() }));
vi.mock('../services/settings', () => ({
  getSettings: vi.fn(),
  saveSettingsPatch: vi.fn(),
  saveLegalAcceptance: vi.fn(),
  setupLibrary: vi.fn(),
  proposeLibraryPath: vi.fn(),
}));

const { getSettings, saveLegalAcceptance } = await import('../services/settings');
const { legalVersion } = await import('../services/legal');
const { settings, initSettings } = await import('./settings');
const { showLegalAcceptance, respondLegalAcceptance, loadLegalVersion, currentLegalVersion, legalVersionFailed } =
  await import('./legalAcceptance');

function makeSettings(legalAcceptedVersion: string): Settings {
  return {
    uiScale: 1,
    animationsEnabled: true,
    legalAcceptedVersion,
  } as Settings;
}

async function load(legalAcceptedVersion: string) {
  vi.mocked(getSettings).mockResolvedValue(makeSettings(legalAcceptedVersion));
  await initSettings();
}

describe('legalAcceptance', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    settings.set(null);
    currentLegalVersion.set(null);
    legalVersionFailed.set(false);
  });

  it('показывает экран, когда условия ещё не приняты', async () => {
    await load('');
    vi.mocked(legalVersion).mockResolvedValue('2026-10-10');
    await loadLegalVersion();
    expect(get(showLegalAcceptance)).toBe(true);
  });

  it('не показывает экран, когда принята текущая версия', async () => {
    await load('2026-10-10');
    vi.mocked(legalVersion).mockResolvedValue('2026-10-10');
    await loadLegalVersion();
    expect(get(showLegalAcceptance)).toBe(false);
  });

  it('показывает экран снова, когда версия документов изменилась', async () => {
    await load('2026-10-10');
    vi.mocked(legalVersion).mockResolvedValue('2026-11-01');
    await loadLegalVersion();
    expect(get(showLegalAcceptance)).toBe(true);
  });

  it('не показывает экран, пока версия не загружена', async () => {
    await load('');
    expect(get(showLegalAcceptance)).toBe(false);
  });

  it('не показывает экран, пока не загружены настройки', async () => {
    vi.mocked(legalVersion).mockResolvedValue('2026-10-10');
    await loadLegalVersion();
    expect(get(showLegalAcceptance)).toBe(false);
  });

  it('при ошибке получения версии показывает экран, а не пропускает принятие', async () => {
    await load('2026-10-10');
    vi.mocked(legalVersion).mockRejectedValue(new Error('boom'));
    await loadLegalVersion();
    expect(get(currentLegalVersion)).toBeNull();
    expect(get(legalVersionFailed)).toBe(true);
    expect(get(showLegalAcceptance)).toBe(true);
  });

  it('повторная загрузка версии снимает ошибку', async () => {
    await load('2026-10-10');
    vi.mocked(legalVersion).mockRejectedValueOnce(new Error('boom'));
    await loadLegalVersion();
    vi.mocked(legalVersion).mockResolvedValue('2026-10-10');
    await loadLegalVersion();
    expect(get(legalVersionFailed)).toBe(false);
    expect(get(showLegalAcceptance)).toBe(false);
  });

  it('принятие сохраняет текущую версию и обновляет стор', async () => {
    await load('');
    vi.mocked(legalVersion).mockResolvedValue('2026-10-10');
    await loadLegalVersion();
    const saved = makeSettings('2026-10-10');
    vi.mocked(saveLegalAcceptance).mockResolvedValue(saved);

    await respondLegalAcceptance();

    expect(saveLegalAcceptance).toHaveBeenCalledTimes(1);
    expect(saveLegalAcceptance).toHaveBeenCalledWith('2026-10-10');
    expect(get(settings)).toEqual(saved);
    expect(get(showLegalAcceptance)).toBe(false);
  });

  it('не сохраняет, пока версия не получена', async () => {
    await load('');
    await expect(respondLegalAcceptance()).rejects.toThrow('legal version not loaded');
    expect(saveLegalAcceptance).not.toHaveBeenCalled();
  });

  it('не засчитывает принятие, если сохранение упало', async () => {
    await load('');
    vi.mocked(legalVersion).mockResolvedValue('2026-10-10');
    await loadLegalVersion();
    vi.mocked(saveLegalAcceptance).mockRejectedValue(new Error('disk full'));

    await expect(respondLegalAcceptance()).rejects.toThrow('disk full');

    expect(get(settings)?.legalAcceptedVersion).toBe('');
    expect(get(showLegalAcceptance)).toBe(true);
  });
});
