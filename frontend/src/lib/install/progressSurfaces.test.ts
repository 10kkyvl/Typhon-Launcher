import { describe, expect, it } from 'vitest';
import { downloadsPageSource } from '../testing/downloadsPageParts';
import { frontendSources } from '../testing/sources';

// A screen split into parts is read whole: the helper may be imported by any of its parts.
const SCREENS: Record<string, string> = { 'routes/downloads/': downloadsPageSource };

const SURFACES = [
  'lib/components/InstallModal.svelte',
  'routes/downloads/',
  'routes/game/GameDetails.svelte',
  'lib/bigpicture/components/DownloadsPage.svelte',
  'lib/bigpicture/components/GamePage.svelte',
  'lib/stores/activity.ts',
  'lib/stores/notifications.ts',
];

describe('install progress surfaces', () => {
  it.each(SURFACES)('%s decides how to draw progress through the shared helper', (path) => {
    const source = SCREENS[path] ?? frontendSources[path];
    expect(source, path).toBeTypeOf('string');
    expect(source).toMatch(/from '[^']*install\/progress'/);
  });

  it('keeps every other reader of an installation total behind that helper', () => {
    const readers = Object.entries(frontendSources)
      .filter(([path, source]) => /\binstall(?:ation)?\.bytesTotal\b/.test(source) && path !== 'lib/install/progress.ts')
      .map(([path]) => path)
      .filter((path) => !/install\/progress'/.test(frontendSources[path]));
    expect(readers).toEqual([]);
  });
});
