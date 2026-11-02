import { describe, expect, it } from 'vitest';
import {
  buildKind,
  buildLabel,
  consoleGroupStart,
  consoleOrigin,
  orderReleases,
  relationKey,
  releaseBadge,
  releaseOrigin,
  repackNote,
  sharedPlatform,
  splitReleases,
} from './releases';

describe('releaseOrigin', () => {
  it('carries the release version so the install records it', () => {
    const origin = releaseOrigin({
      uri: 'magnet:?xt=urn:btih:abc',
      name: 'Game — v1.0.30000 | Portable',
      releaseId: 'r1',
      sourceId: 's1',
      distributionId: 'game-main',
      releaseUploadedAt: '2026-09-09T12:00:00Z',
      gameId: 'g1',
      version: '1.0.30000',
    });
    expect(origin).toEqual({
      releaseId: 'r1',
      sourceId: 's1',
      distributionId: 'game-main',
      releaseUploadedAt: '2026-09-09T12:00:00Z',
      gameId: 'g1',
      version: '1.0.30000',
    });
  });

  it('omits an unknown version instead of recording an empty one', () => {
    const origin = releaseOrigin({
      uri: 'magnet:?xt=urn:btih:abc',
      name: 'Game',
      releaseId: 'r1',
      sourceId: 's1',
      gameId: 'g1',
      version: '',
    });
    expect(origin).toEqual({ releaseId: 'r1', sourceId: 's1', gameId: 'g1' });
  });
});

describe('releaseBadge', () => {
  const base = { releaseId: 'r2', currentReleaseId: 'r1', targetReleaseId: 'r2', isNew: false };

  it('marks the installed release', () => {
    expect(releaseBadge({ ...base, releaseId: 'r1', updateKind: 'update' })).toBe('installed');
  });

  it('calls a target an update only when the resolver claims one', () => {
    expect(releaseBadge({ ...base, updateKind: 'update' })).toBe('update');
  });

  it('shows an updated stable record as an update, not as already installed', () => {
    expect(
      releaseBadge({ releaseId: 'r1', currentReleaseId: 'r1', targetReleaseId: 'r1', updateKind: 'update', isNew: false }),
    ).toBe('update');
  });

  it('shows a changed-version-scheme stable record as a new release', () => {
    expect(
      releaseBadge({
        releaseId: 'r1',
        currentReleaseId: 'r1',
        targetReleaseId: 'r1',
        updateKind: 'new_release',
        isNew: false,
      }),
    ).toBe('new-release');
  });

  it('calls a merely different build a new release', () => {
    expect(releaseBadge({ ...base, updateKind: 'new_release' })).toBe('new-release');
  });

  it('ignores the target when nothing is available', () => {
    expect(releaseBadge({ ...base, updateKind: 'none' })).toBe('none');
  });

  it('falls back to the new-in-feed mark', () => {
    expect(releaseBadge({ ...base, releaseId: 'r3', updateKind: 'update', isNew: true })).toBe('new');
  });
});

describe('buildKind', () => {
  it('marks a release delivered as a ready folder', () => {
    expect(buildKind({ tags: ['portable'] })).toBe('portable');
  });

  it('names where the files came from when nothing shapes the delivery', () => {
    expect(buildKind({ tags: ['gog'] })).toBe('gog');
    expect(buildKind({ tags: ['archive'] })).toBe('archive');
    expect(buildKind({ tags: ['p2p'] })).toBe('p2p');
  });

  it('prefers the delivered form over where the files came from', () => {
    expect(buildKind({ tags: ['p2p', 'portable'] })).toBe('portable');
    expect(buildKind({ tags: ['p2p', 'license'] })).toBe('license');
  });

  it('leaves who packed it to the source line', () => {
    expect(buildKind({ tags: ['repack'] })).toBe('none');
  });

  it('says nothing about a release it cannot classify', () => {
    expect(buildKind({ tags: ['x64'] })).toBe('none');
    expect(buildKind({})).toBe('none');
  });
});

describe('buildLabel', () => {
  it('gives every build kind a caption', () => {
    const kinds = [
      'portable',
      'archive',
      'steam-rip',
      'gog',
      'license',
      'early-access',
      'demo',
      'p2p',
    ] as const;
    for (const kind of kinds) expect(buildLabel(kind)).toBeTruthy();
  });

  it('leaves an unclassified release without a badge', () => {
    expect(buildLabel('none')).toBeNull();
  });
});

describe('repackNote', () => {
  it('signs a repack with the packer the feed named', () => {
    expect(repackNote({ tags: ['repack'], repacker: 'xatab' }, 'Репак')).toBe('Репак Xatab');
  });

  it('spells a packer the way the feed knows them', () => {
    expect(repackNote({ tags: ['repack'], repacker: 'mechanics' }, 'Репак')).toBe('Репак Механики');
  });

  it('still calls it a repack when nobody signed it', () => {
    expect(repackNote({ tags: ['repack'], repacker: '' }, 'Репак')).toBe('Репак');
  });

  it('names the author of a build that is not a repack', () => {
    expect(repackNote({ tags: ['steam-rip'], repacker: 'chovka' }, 'Репак')).toBe('Chovka');
  });

  it('leaves an unattributed release alone', () => {
    expect(repackNote({ tags: ['archive'], repacker: '' }, 'Репак')).toBe('');
    expect(repackNote({}, 'Репак')).toBe('');
  });
});

describe('relationKey', () => {
  it('maps every relation to its label', () => {
    expect(relationKey('original')).toBe('games.relationOriginal');
    expect(relationKey('base')).toBe('games.relationBase');
    expect(relationKey('remake')).toBe('games.relationRemake');
    expect(relationKey('remaster')).toBe('games.relationRemaster');
    expect(relationKey('expanded')).toBe('games.relationExpanded');
    expect(relationKey('port')).toBe('games.relationPort');
    expect(relationKey('edition')).toBe('games.relationEdition');
    expect(relationKey('update')).toBe('games.relationUpdate');
  });

  it('falls back to the edition label for an unknown relation', () => {
    expect(relationKey('sequel')).toBe('games.relationEdition');
  });
});

describe('console releases', () => {
  const pc = { release: { id: 'a', title: 'G', platform: undefined } };
  const ps2 = { release: { id: 'b', title: 'G', platform: 'ps2' as const } };
  const ps1 = { release: { id: 'c', title: 'G', platform: 'ps1' as const } };

  it('splits the console images from the PC releases', () => {
    expect(splitReleases([ps2, pc, ps1])).toEqual({ pc: [pc], console: [ps2, ps1] });
  });

  it('puts PC releases first and keeps the order inside each group', () => {
    expect(orderReleases([ps2, pc, ps1]).map((g) => g.release.id)).toEqual(['a', 'b', 'c']);
  });

  it('names a platform only when all console images share it', () => {
    expect(sharedPlatform([ps2, ps2])).toBe('ps2');
    expect(sharedPlatform([ps2, ps1])).toBe('');
    expect(sharedPlatform([])).toBe('');
  });

  it('puts the console and the card into the origin of an image download', () => {
    expect(consoleOrigin({ platform: 'ps2', title: 'Raw' }, ' Game ', 'https://img/c.jpg')).toEqual({
      platform: 'ps2',
      title: 'Game',
      cover: 'https://img/c.jpg',
    });
  });

  it('falls back to the release title and drops a cover that is not a web address', () => {
    expect(consoleOrigin({ platform: 'psp', title: 'Raw' }, '', 'C:\covers\g.png')).toEqual({
      platform: 'psp',
      title: 'Raw',
    });
  });

  it('adds nothing to the origin of a PC release', () => {
    expect(consoleOrigin({ title: 'Raw' }, 'Game', 'https://img/c.jpg')).toEqual({});
  });
});

describe('consoleGroupStart', () => {
  const pc = { release: { id: 'a', platform: undefined } };
  const ps2 = { release: { id: 'b', platform: 'ps2' as const } };

  it('opens the console group after the PC releases', () => {
    expect(consoleGroupStart([ps2, pc])).toBe(1);
    expect(consoleGroupStart([pc, pc, ps2, ps2])).toBe(2);
  });

  it('has no heading when only one kind is listed', () => {
    expect(consoleGroupStart([ps2, ps2])).toBe(-1);
    expect(consoleGroupStart([pc])).toBe(-1);
    expect(consoleGroupStart([])).toBe(-1);
  });
});
