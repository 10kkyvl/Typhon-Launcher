import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

const SURFACES = [
  'lib/components/InstallModal.svelte',
  'routes/downloads/Downloads.svelte',
  'routes/game/GameDetails.svelte',
  'lib/bigpicture/components/DownloadsPage.svelte',
  'lib/bigpicture/components/GamePage.svelte',
  'lib/stores/activity.ts',
  'lib/stores/notifications.ts',
];

describe('install progress surfaces', () => {
  it.each(SURFACES)('%s decides how to draw progress through the shared helper', (path) => {
    const source = frontendSources[path];
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
