import { describe, expect, it } from 'vitest';
import source from './Library.svelte?raw';
import { libraryErrorText } from '../../lib/library/errors';

describe('Library error toasts', () => {
  it('never shows the raw backend error message to the user', () => {
    expect(source).not.toMatch(/err\.message/);
    expect(source).not.toMatch(/instanceof Error/);
  });

  const sites: Array<{ name: string; snippet: string }> = [
    { name: 'loadCatalogGames', snippet: "toast(metadataErrorText(err, msg('games.libraryLoadGamesError')), 'danger');" },
    { name: 'toggleRun', snippet: "toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');" },
  ];

  it.each(sites)('$name resolves the error through a translated catalog lookup, not raw text', ({ snippet }) => {
    expect(source).toContain(snippet);
  });

  it('resolves an unrecognised library error code through the direct catalog key with a translated fallback', () => {
    expect(source).toMatch(/import \{ libraryErrorText \} from '\.\.\/\.\.\/lib\/library\/errors';/);
    expect(libraryErrorText(new Error('typhon:library.no_such_code'), 'fallback')).toBe('fallback');
    expect(libraryErrorText(new Error('typhon:library.game_not_found'), 'fallback')).not.toBe('fallback');
  });
});
