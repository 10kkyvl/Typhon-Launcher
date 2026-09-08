import { describe, expect, it } from 'vitest';
import source from './Library.svelte?raw';

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
    expect(source).toMatch(
      /function libraryErrorText\(err: unknown, fallback: string\): string \{\s*const code = errorCode\(err\);\s*return hasMessage\(code\) \? msg\(code\) : fallback;\s*\}/,
    );
  });
});
