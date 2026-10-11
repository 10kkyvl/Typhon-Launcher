import { render } from 'svelte/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Settings } from '../services/settings';
import source from './TelemetryConsentScreen.svelte?raw';

globalThis.document = {
  documentElement: {
    style: { setProperty: () => {} },
    classList: { toggle: () => {} },
  },
} as unknown as Document;

vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/toasts', () => ({ toast: vi.fn() }));
vi.mock('../services/settings', () => ({
  getSettings: vi.fn(),
  saveSettingsPatch: vi.fn(),
  saveConsent: vi.fn(),
  setupLibrary: vi.fn(),
  proposeLibraryPath: vi.fn(),
}));

const { settings } = await import('../stores/settings');
const { default: TelemetryConsentScreen } = await import('./TelemetryConsentScreen.svelte');
const { locale } = await import('../i18n');

function stored(version: number, usage: boolean, diagnostics: boolean): Settings {
  return {
    uiScale: 1,
    animationsEnabled: true,
    anonymousUsageStats: usage,
    anonymousDiagnostics: diagnostics,
    telemetryConsentVersion: version,
  } as Settings;
}

function toggles(html: string, label: string): boolean {
  const tag = new RegExp(`<button[^>]*aria-label="${label}"[^>]*>`).exec(html)?.[0];
  expect(tag, label).toBeDefined();
  const checked = /aria-checked="(true|false)"/.exec(tag ?? '')?.[1];
  expect(checked, label).toBeDefined();
  return checked === 'true';
}

function screen() {
  const html = render(TelemetryConsentScreen).body;
  return {
    html,
    usage: toggles(html, 'Анонимная статистика использования'),
    diagnostics: toggles(html, 'Анонимная диагностика'),
  };
}

beforeEach(() => {
  locale.set('ru');
  settings.set(null);
});

describe('TelemetryConsentScreen', () => {
  it('starts with both switches off on a fresh install', () => {
    settings.set(stored(0, false, false));
    const view = screen();
    expect(view.usage).toBe(false);
    expect(view.diagnostics).toBe(false);
  });

  it('ignores stored switch values while nothing has been answered yet', () => {
    settings.set(stored(0, true, true));
    const view = screen();
    expect(view.usage).toBe(false);
    expect(view.diagnostics).toBe(false);
  });

  it('repeats the previous answer on a re-prompt', () => {
    settings.set(stored(2, true, true));
    const view = screen();
    expect(view.usage).toBe(true);
    expect(view.diagnostics).toBe(true);
  });

  it('preselects each switch from its own previous answer', () => {
    settings.set(stored(2, false, true));
    const view = screen();
    expect(view.usage).toBe(false);
    expect(view.diagnostics).toBe(true);

    settings.set(stored(1, true, false));
    const other = screen();
    expect(other.usage).toBe(true);
    expect(other.diagnostics).toBe(false);
  });

  it('shows the heartbeat example next to the other examples', () => {
    settings.set(stored(0, false, false));
    const { html } = screen();
    expect(html).toContain('Сигнал «лаунчер онлайн»');
    expect(html).toContain('"state": "idle"');
    expect(html).toContain('installation_id и game_id');
  });

  it('wires decline to a (false, false) answer and save to the switches as shown', () => {
    expect(source).toMatch(/onclick=\{\(\) => respond\(false, false\)\}[^]*?telemetryConsentDecline/);
    expect(source).toMatch(/onclick=\{\(\) => respond\(usageStats, diagnostics\)\}[^]*?telemetryConsentAccept/);
    expect(source).toContain('await respondTelemetryConsent(sendUsageStats, sendDiagnostics);');
  });
});
