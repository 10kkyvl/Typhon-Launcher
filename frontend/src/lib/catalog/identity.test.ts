import { describe, expect, it } from 'vitest';
import { mergeCatalogDisplay } from './display';
import { identityEvidenceChanged, matchesCatalogIdentity } from './identity';
import type { CatalogGame } from '../services/sources';

const game = (id: string, patch: Partial<CatalogGame> = {}): CatalogGame => ({
  id,
  title: 'Same title',
  sortTitle: 'same title',
  createdAt: '',
  ...patch,
});

describe('catalog identity evidence', () => {
  it('ignores mutable metadata changes', () => {
    const before = game('canonical', { externalIds: { igdb: '375995' } });
    const after = game('canonical', {
      title: 'Updated title',
      developer: 'Andrei',
      coverUrl: 'new-cover',
      externalIds: { igdb: '375995' },
    });

    expect(identityEvidenceChanged(before, after)).toBe(false);
  });

  it('detects provider links without using title similarity', () => {
    const catalog = game('local-steam', { externalIds: { steam: '3946730' } });
    const update = game('canonical-igdb', {
      externalIds: { igdb: '375995' },
      providerLinks: { steam: ['3946730'] },
    });
    const unrelated = game('other', { externalIds: { steam: '999' } });

    expect(matchesCatalogIdentity(catalog, update)).toBe(true);
    expect(matchesCatalogIdentity(catalog, unrelated)).toBe(false);
  });

  it('detects an explicit alias link', () => {
    expect(matchesCatalogIdentity(game('canonical', { aliasIds: ['old-steam'] }), game('old-steam'))).toBe(true);
  });
});

describe('catalog display merge', () => {
  it('keeps the Browse developer when metadata has an empty developer', () => {
    const browse = game('canonical', { developer: 'Andrei' });
    const staleMetadata = game('canonical', { developer: '' });

    expect(mergeCatalogDisplay(browse, staleMetadata).developer).toBe('Andrei');
  });
});
