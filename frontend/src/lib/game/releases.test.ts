import { describe, expect, it } from 'vitest';
import { buildKind, buildLabel, releaseBadge, releaseOrigin, repackerLabel } from './releases';

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
    expect(buildKind({ tags: ['portable'], repacker: '' })).toBe('portable');
  });

  it('marks a repack the feed did not attribute to anyone', () => {
    expect(buildKind({ tags: ['repack'], repacker: '' })).toBe('repack');
  });

  it('leaves the repack mark to the repacker badge', () => {
    expect(buildKind({ tags: ['repack'], repacker: 'xatab' })).toBe('none');
  });

  it('prefers the delivered form over the packing method', () => {
    expect(buildKind({ tags: ['repack', 'portable'], repacker: 'xatab' })).toBe('portable');
  });

  it('names where the files came from when nobody repacked them', () => {
    expect(buildKind({ tags: ['gog'], repacker: '' })).toBe('gog');
    expect(buildKind({ tags: ['archive'], repacker: '' })).toBe('archive');
    expect(buildKind({ tags: ['p2p'], repacker: '' })).toBe('p2p');
  });

  it('prefers what the build is over where it came from', () => {
    expect(buildKind({ tags: ['archive', 'portable'], repacker: '' })).toBe('portable');
    expect(buildKind({ tags: ['p2p', 'license'], repacker: '' })).toBe('license');
  });

  it('says nothing about a release it cannot classify', () => {
    expect(buildKind({ tags: ['x64'], repacker: '' })).toBe('none');
    expect(buildKind({})).toBe('none');
  });
});

describe('buildLabel', () => {
  it('gives every build kind a caption', () => {
    const kinds = [
      'portable',
      'repack',
      'steam-rip',
      'gog',
      'license',
      'early-access',
      'demo',
      'p2p',
      'archive',
    ] as const;
    for (const kind of kinds) expect(buildLabel(kind)).toBeTruthy();
  });

  it('leaves an unclassified release without a badge', () => {
    expect(buildLabel('none')).toBeNull();
  });
});

describe('repackerLabel', () => {
  it('signs a repacker the way the feed names them', () => {
    expect(repackerLabel('mechanics')).toBe('МЕХАНИКИ');
  });

  it('falls back to the slug itself', () => {
    expect(repackerLabel('xatab')).toBe('XATAB');
  });
});
