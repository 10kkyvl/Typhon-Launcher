import { describe, expect, it } from 'vitest';
import { releaseBadge, releaseOrigin } from './releases';

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
