import { get } from 'svelte/store';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { HandoffInfo } from './lib/services/selfupdate';
import type { AppInfo } from './lib/services/system';

let overlay = false;
let info: () => Promise<AppInfo>;
let handoff: HandoffInfo | null = null;
const seenAtMount: { platform?: string; controls?: string }[] = [];
const bootAtMount: (string | null)[] = [];
// The boot colour index.html puts in <head>; `removed` once main.ts drops it.
const boot = { textContent: 'html, body { background: #090d17; }', removed: false, remove: () => (boot.removed = true) };

vi.mock('@fontsource-variable/onest', () => ({}));
vi.mock('@fontsource-variable/jetbrains-mono', () => ({}));
vi.mock('./styles/tokens.css', () => ({}));
vi.mock('./styles/shell.css', () => ({}));
vi.mock('./styles/global.css', () => ({}));
vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) }, Window: {} }));
vi.mock('./lib/services/backend', () => ({ inWails: true }));
vi.mock('./lib/services/diagnostics', () => ({ installDiagnostics: vi.fn() }));
vi.mock('./lib/services/overlay', () => ({ isOverlayWindow: () => overlay }));
vi.mock('./lib/components/ThemeGuard', () => ({ mountThemeGuard: vi.fn() }));
vi.mock('./lib/stores/theme', () => ({ initTheme: vi.fn(async () => {}), resetAppearance: vi.fn() }));
vi.mock('./lib/stores/settings', () => ({ initSettings: vi.fn(async () => {}) }));
vi.mock('./lib/stores/toasts', () => ({ toast: vi.fn() }));
vi.mock('./lib/services/system', () => ({ appInfo: vi.fn(() => info()) }));
// As the real one: a successor takes the show over before it answers.
vi.mock('./lib/stores/handoff', () => ({
  loadHandoff: vi.fn(async () => {
    if (handoff) (await import('./lib/stores/updateShow')).takeOverShow(handoff.phaseStart, handoff.version);
    return handoff;
  }),
}));
vi.mock('./App.svelte', () => ({ default: 'App' }));
vi.mock('./overlay/Overlay.svelte', () => ({ default: 'Overlay' }));
vi.mock('svelte', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  mount: vi.fn(async () => {
    bootAtMount.push(boot.removed ? null : boot.textContent);
    const store = await import('./lib/stores/window');
    seenAtMount.push({ platform: get(store.platform), controls: get(store.windowControls) });
  }),
}));

globalThis.document = {
  getElementById: (id: string) => (id === 'boot-bg' ? (boot.removed ? null : boot) : {}),
  documentElement: { classList: { add: vi.fn() } },
} as unknown as Document;
globalThis.window = { addEventListener: vi.fn() } as unknown as Window & typeof globalThis;

async function start() {
  vi.resetModules();
  seenAtMount.length = 0;
  bootAtMount.length = 0;
  await import('./main');
  const { mount } = await import('svelte');
  await vi.waitFor(() => expect(seenAtMount).toHaveLength(1));
  return { mount: vi.mocked(mount), system: vi.mocked((await import('./lib/services/system')).appInfo) };
}

beforeEach(() => {
  vi.clearAllMocks();
  overlay = false;
  handoff = null;
  boot.textContent = 'html, body { background: #090d17; }';
  boot.removed = false;
  info = async () => ({ platform: 'windows' }) as AppInfo;
});

describe('the boot colour', () => {
  it('in an ordinary start, stands until the app is mounted, then goes', async () => {
    await start();
    expect(bootAtMount[0]).toContain('#090d17');
    expect(boot.removed).toBe(true);
  });

  it('in a successor of a seamless restart, is the colour handed over, painted before the app mounts and kept while the show runs', async () => {
    handoff = { active: true, phaseStart: 1, version: '1.5.0', background: '#f4f1ea' };
    await start();
    expect(bootAtMount[0]).toBe('html, body { background: #f4f1ea; }');
    expect(boot.removed).toBe(false);
    (await import('./lib/stores/updateShow')).endUpdateShow();
    expect(boot.removed).toBe(true);
  });

  it('never takes a colour that is not one', async () => {
    handoff = { active: true, phaseStart: 1, version: '1.5.0', background: 'red; } body { display: none' };
    await start();
    expect(bootAtMount[0]).toContain('#090d17');
  });
});

describe('startup platform', () => {
  it('knows the platform before the main window mounts', async () => {
    info = () => new Promise((resolve) => setTimeout(() => resolve({ platform: 'linux' } as AppInfo), 20));
    await start();
    expect(seenAtMount[0]).toEqual({ platform: 'linux', controls: 'buttons' });
  });

  it('mounts with the window buttons when the platform lookup fails', async () => {
    info = async () => {
      throw new Error('binding down');
    };
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    await start();
    expect(seenAtMount[0].controls).toBe('buttons');
    expect(error).toHaveBeenCalled();
    error.mockRestore();
  });

  it('does not ask for the platform in the overlay window', async () => {
    overlay = true;
    const { system } = await start();
    expect(system).not.toHaveBeenCalled();
  });
});
