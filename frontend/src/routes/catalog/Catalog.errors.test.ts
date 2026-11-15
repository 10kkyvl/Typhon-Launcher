import { describe, expect, it } from 'vitest';
import results from './CatalogResults.svelte?raw';
import pageData from './catalogPageData.svelte.ts?raw';
import pages from './catalogPages.svelte.ts?raw';

describe('Catalog error toasts', () => {
  // Every file of the page that catches an error from Go.
  it.each([
    ['CatalogResults.svelte', results],
    ['catalogPageData.svelte.ts', pageData],
    ['catalogPages.svelte.ts', pages],
  ])('%s never shows the raw backend error message to the user', (_name, source) => {
    expect(source).not.toMatch(/err\.message/);
    expect(source).not.toMatch(/instanceof Error/);
  });

  const sites: Array<{ name: string; source: string; snippet: string }> = [
    { name: 'toggleRun', source: results, snippet: "toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');" },
    { name: 'toggleFavorite', source: pageData, snippet: "toast(libraryErrorText(err, msg('games.errorFavoriteFailed')), 'danger');" },
  ];

  it.each(sites)('$name resolves the error through a translated catalog lookup, not raw text', ({ source, snippet }) => {
    expect(source).toContain(snippet);
  });

  it('resolves an unrecognised library error code through the direct catalog key with a translated fallback', () => {
    expect(pageData).toMatch(
      /function libraryErrorText\(err: unknown, fallback: string\): string \{\s*const code = errorCode\(err\);\s*return hasMessage\(code\) \? msg\(code\) : fallback;\s*\}/,
    );
  });
});
