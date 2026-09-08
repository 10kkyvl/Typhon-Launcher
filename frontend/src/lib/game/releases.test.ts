import { describe, expect, it } from 'vitest';
import { buildKind, buildLabel, releaseBadge, releaseOrigin, repackNote } from './releases';

describe('releaseOrigin', () => {
  it('carries the release version so the install records it', () => {
    const origin = releaseOrigin({
      uri: 'magnet:?xt=urn:btih:abc',
      name: 'Game — v1.0.30000 | Portable',
      releaseId: 'r1',
      sourceId: 's1',
      gameId: 'g1',
      version: '1.0.30000',
    });
    expect(origin).toEqual({ releaseId: 'r1', sourceId: 's1', gameId: 'g1', version: '1.0.30000' });
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
