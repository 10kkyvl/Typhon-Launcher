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
  saveSettingsPatch: vi.fn(),
  saveConsent: vi.fn(),
  setupLibrary: vi.fn(),
  proposeLibraryPath: vi.fn(),
}));

const { getSettings, saveConsent } = await import('../services/settings');
const { settings, initSettings } = await import('./settings');
const { showTelemetryConsent, respondTelemetryConsent, currentTelemetryConsent, consentPreselection } = await import('./telemetryConsent');

function makeSettings(telemetryConsentVersion: number): Settings {
  return {
    uiScale: 1,
    animationsEnabled: true,
    sourcesNoticeAccepted: true,
    anonymousUsageStats: false,
    anonymousDiagnostics: true,
    telemetryConsentVersion,
  } as Settings;
}

async function load(telemetryConsentVersion: number) {
  vi.mocked(getSettings).mockResolvedValue(makeSettings(telemetryConsentVersion));
  await initSettings();
}

describe('telemetryConsent', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    settings.set(null);
  });

  it('не требует согласия, пока настройки не загружены', () => {
    expect(get(showTelemetryConsent)).toBe(false);
  });

  it('требует согласия, когда telemetryConsentVersion равен нулю', async () => {
    await load(0);
    expect(get(showTelemetryConsent)).toBe(true);
  });

  it('не требует согласия повторно, когда ответ дан на текущую версию', async () => {
    await load(currentTelemetryConsent);
    expect(get(showTelemetryConsent)).toBe(false);
  });

  // Ответ, данный на старый текст, покрывает только то, что тот текст описывал.
  // Вторая версия добавила отчёт о совместимости с чипом и версией системы, и
  // растянуть на него согласие первой значило бы собрать их без спроса.
  it('спрашивает заново, когда ответ дан на прошлую версию текста', async () => {
    await load(currentTelemetryConsent - 1);
    expect(get(showTelemetryConsent)).toBe(true);
  });

  it('принятие сохраняет ответ с ожидаемыми аргументами и обновляет стор', async () => {
    await load(0);
    const saved = makeSettings(currentTelemetryConsent);
    saved.anonymousUsageStats = true;
    saved.anonymousDiagnostics = true;
    vi.mocked(saveConsent).mockResolvedValue(saved);

    await respondTelemetryConsent(true, true);

    expect(saveConsent).toHaveBeenCalledTimes(1);
    expect(saveConsent).toHaveBeenCalledWith(true, true);
    expect(get(settings)).toEqual(saved);
    expect(get(showTelemetryConsent)).toBe(false);
  });

  it('отказ тоже фиксируется как ответ, а не просто закрывает экран', async () => {
    await load(0);
    const saved = makeSettings(currentTelemetryConsent);
    saved.anonymousUsageStats = false;
    saved.anonymousDiagnostics = false;
    vi.mocked(saveConsent).mockResolvedValue(saved);

    await respondTelemetryConsent(false, false);

    expect(saveConsent).toHaveBeenCalledTimes(1);
    expect(saveConsent).toHaveBeenCalledWith(false, false);
    expect(get(settings)).toEqual(saved);
    expect(get(showTelemetryConsent)).toBe(false);
  });

  it('третья версия: ответ второй версии больше не покрывает пинг с идентификатором установки', async () => {
    expect(currentTelemetryConsent).toBe(3);
    await load(2);
    expect(get(showTelemetryConsent)).toBe(true);
  });

  describe('consentPreselection', () => {
    it('на свежей установке не отмечает ничего, что бы ни лежало в настройках', () => {
      expect(consentPreselection(null)).toEqual({ usageStats: false, diagnostics: false });
      expect(consentPreselection(makeSettings(0))).toEqual({ usageStats: false, diagnostics: false });
      const stale = makeSettings(0);
      stale.anonymousUsageStats = true;
      stale.anonymousDiagnostics = true;
      expect(consentPreselection(stale)).toEqual({ usageStats: false, diagnostics: false });
    });

    it('при повторном вопросе повторяет прошлый ответ по каждому переключателю отдельно', () => {
      const both = makeSettings(2);
      both.anonymousUsageStats = true;
      both.anonymousDiagnostics = true;
      expect(consentPreselection(both)).toEqual({ usageStats: true, diagnostics: true });

      const onlyUsage = makeSettings(1);
      onlyUsage.anonymousUsageStats = true;
      onlyUsage.anonymousDiagnostics = false;
      expect(consentPreselection(onlyUsage)).toEqual({ usageStats: true, diagnostics: false });

      const onlyDiagnostics = makeSettings(2);
      onlyDiagnostics.anonymousUsageStats = false;
      onlyDiagnostics.anonymousDiagnostics = true;
      expect(consentPreselection(onlyDiagnostics)).toEqual({ usageStats: false, diagnostics: true });
    });
  });

  it('не засчитывает согласие, если сохранение упало', async () => {
    await load(0);
    vi.mocked(saveConsent).mockRejectedValue(new Error('disk full'));

    await expect(respondTelemetryConsent(false, true)).rejects.toThrow('disk full');

    expect(get(settings)?.telemetryConsentVersion).toBe(0);
    expect(get(showTelemetryConsent)).toBe(true);
  });
});
