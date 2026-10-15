import { describe, expect, it, vi } from 'vitest';

vi.mock('./backend', () => ({ inWails: false }));
vi.mock('@wailsio/runtime', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Events: { On: vi.fn(() => vi.fn()) },
}));

import { SCREENSHOT_HOTKEYS, deleteShot, listShots, openShotsFolder, screenshotsStatus } from './screenshots';
import { getStats } from './playlog';
import { getSettings } from './settings';
import { goSources } from '../testing/sources';

describe('screenshot hotkeys', () => {
  it('offers the hotkeys Go accepts, in the same order', () => {
    const go = goSources['internal/settings/settings.go'];
    const names = /func ScreenshotHotkeys\(\) \[\]string \{\s*return \[\]string\{([^}]*)\}/.exec(go)?.[1] ?? '';
    const constants = new Map(
      [...go.matchAll(/^\t(ScreenshotHotkey\w+)\s*=\s*"([^"]+)"/gm)].map((match) => [match[1], match[2]]),
    );
    const ids = names.split(',').map((name) => constants.get(name.trim()));
    expect(ids.length).toBeGreaterThan(1);
    expect([...SCREENSHOT_HOTKEYS]).toEqual(ids);
  });

  it('defaults the browser fallback to the first hotkey', async () => {
    const settings = await getSettings();
    expect(settings.screenshotsEnabled).toBe(true);
    expect(settings.screenshotHotkey).toBe(SCREENSHOT_HOTKEYS[0]);
  });
});

describe('outside the desktop shell', () => {
  it('lists no shots and reports the default status', async () => {
    expect(await listShots('g1')).toEqual([]);
    expect(await screenshotsStatus()).toMatchObject({ enabled: true, hotkey: 'F12' });
  });

  it('refuses actions that need the backend instead of pretending they worked', async () => {
    await expect(deleteShot('g1', 'a.png')).rejects.toThrow();
    await expect(openShotsFolder('g1')).rejects.toThrow();
    await expect(getStats('week')).rejects.toThrow();
  });
});
