import { get } from 'svelte/store';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { AppInfo } from './lib/services/system';

let overlay = false;
let info: () => Promise<AppInfo>;
const seenAtMount: { platform?: string; controls?: string }[] = [];

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
vi.mock('./App.svelte', () => ({ default: 'App' }));
vi.mock('./overlay/Overlay.svelte', () => ({ default: 'Overlay' }));
vi.mock('svelte', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  mount: vi.fn(async () => {
    const store = await import('./lib/stores/window');
    seenAtMount.push({ platform: get(store.platform), controls: get(store.windowControls) });
  }),
}));

globalThis.document = {
  getElementById: () => ({}),
  documentElement: { classList: { add: vi.fn() } },
} as unknown as Document;
globalThis.window = { addEventListener: vi.fn() } as unknown as Window & typeof globalThis;

async function start() {
  vi.resetModules();
  seenAtMount.length = 0;
  await import('./main');
  const { mount } = await import('svelte');
  await vi.waitFor(() => expect(seenAtMount).toHaveLength(1));
  return { mount: vi.mocked(mount), system: vi.mocked((await import('./lib/services/system')).appInfo) };
}

beforeEach(() => {
  vi.clearAllMocks();
  overlay = false;
  info = async () => ({ platform: 'windows' }) as AppInfo;
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
