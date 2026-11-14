import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { ReleaseGroup } from '../services/sources';
import { user } from '../testing/dom';
import { description } from '../testing/hints';
import ReleaseList from './ReleaseList.svelte';
import releaseListSource from './ReleaseList.svelte?raw';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());

function group(id: string, patch: { version?: string; rawTitle?: string; source?: string; dlcCount?: number } = {}): ReleaseGroup {
  return {
    sourceName: patch.source ?? 'Main feed',
    release: {
      id,
      sourceId: 'fixture',
      rawTitle: patch.rawTitle ?? `Hades ${patch.version ?? ''}`.trim(),
      title: 'Hades',
      normalizedTitle: 'hades',
      canonicalGameId: 'game',
      version: patch.version ?? '',
      size: 1024,
      uploadedAt: null,
      uris: [],
      matchStatus: 'matched',
      matchConfidence: 1,
      matchMethod: 'override',
      availability: 'available',
      firstSeenAt: '',
      lastSeenAt: '',
      createdAt: '',
      dlcCount: patch.dlcCount,
    },
  } as unknown as ReleaseGroup;
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  app.remove();
  vi.restoreAllMocks();
});

const downloads = () => screen.getAllByRole('button', { name: new RegExp(`^${msg('ui.download')}`) });

describe('ReleaseList: each Download names its release', () => {
  it('by version and source, so the same version from two sources reads apart', () => {
    render(ReleaseList, { target: app, props: { groups: [group('a', { version: '1.2' }), group('b', { version: '1.2', source: 'Mirror' })], ondownload: () => {} } });

    expect(downloads().map((button) => button.getAttribute('aria-label'))).toEqual([
      msg('games.blocks.releaseDownload', { release: '1.2', source: 'Main feed' }),
      msg('games.blocks.releaseDownload', { release: '1.2', source: 'Mirror' }),
    ]);
  });

  it('by its full name when it has no version, or its version and source repeat in the list', () => {
    const groups = [
      group('a', { rawTitle: '11F' }),
      group('b', { rawTitle: '11F (+ Windows 7 Fix)' }),
      group('c', { version: '2.0', rawTitle: 'Hades 2.0' }),
      group('d', { version: '2.0', rawTitle: 'Hades 2.0 + Soundtrack' }),
    ];
    render(ReleaseList, { target: app, props: { groups, ondownload: () => {} } });

    const names = downloads().map((button) => button.getAttribute('aria-label'));
    expect(names).toEqual([
      msg('games.blocks.releaseDownload', { release: '11F', source: 'Main feed' }),
      msg('games.blocks.releaseDownload', { release: '11F (+ Windows 7 Fix)', source: 'Main feed' }),
      msg('games.blocks.releaseDownload', { release: 'Hades 2.0', source: 'Main feed' }),
      msg('games.blocks.releaseDownload', { release: 'Hades 2.0 + Soundtrack', source: 'Main feed' }),
    ]);
    expect(new Set(names).size).toBe(names.length);
  });

  it('downloads the release it names', async () => {
    const ondownload = vi.fn();
    const groups = [group('a', { version: '1.2' }), group('b', { version: '1.3' })];
    render(ReleaseList, { target: app, props: { groups, ondownload } });

    await user().click(screen.getByRole('button', { name: msg('games.blocks.releaseDownload', { release: '1.3', source: 'Main feed' }) }));

    expect(ondownload).toHaveBeenCalledWith(groups[1]);
  });
});

describe('ReleaseList: hints and words', () => {
  it('gives the full release name as the Download hint, not as a native title', () => {
    render(ReleaseList, { target: app, props: { groups: [group('a', { version: '1.2', rawTitle: 'Hades v1.2 [Repack]' })], ondownload: () => {} } });

    expect(app.querySelector('[title]')).toBeNull();
    expect(description(downloads()[0])).toBe('Hades v1.2 [Repack]');
  });

  it('says the DLC count in the catalog\'s words', () => {
    render(ReleaseList, { target: app, props: { groups: [group('a', { version: '1.2', dlcCount: 3 })], ondownload: () => {} } });

    expect(app.querySelector('.release-size')?.textContent).toContain(msg('games.blocks.releaseDlc', { count: 3 }));
    // Both catalogs say it the same way today, so the words alone cannot tell a literal from the catalog.
    expect(releaseListSource).not.toMatch(/\bDLC\b/);
  });
});
