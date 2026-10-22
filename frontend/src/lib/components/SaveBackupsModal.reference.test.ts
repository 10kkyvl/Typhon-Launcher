import { describe, expect, it } from 'vitest';
import source from './SaveBackupsModal.svelte?raw';

describe('SaveBackupsModal source link', () => {
  it('opens the wiki page by the catalog id, not the library id', () => {
    expect(source).toContain("await openReferenceLink(id, 'pcgamingwiki');");
    expect(source).toMatch(/const id = canonicalId;\s*if \(!id\) return;/);
    expect(source).not.toContain("openReferenceLink(gameId, 'pcgamingwiki')");
  });

  it('shows a translated error instead of the raw code', () => {
    expect(source).toMatch(/catch \(err\) \{\s*failure = metadataErrorText\(err\);/);
  });
});
