import { describe, expect, it } from 'vitest';

// The `dom` project in vite.config.ts takes exactly the *.dom.test.ts files. A per-file environment
// docblock would put a test in a DOM without that project's setup (cleanup, locale), and a node test
// that imports Testing Library has no document to render into.

const tests = import.meta.glob<string>(['../../**/*.test.ts', '!./domConvention.test.ts'], {
  query: '?raw',
  import: 'default',
  eager: true,
});

const DOCBLOCK = /@vitest-environment\b/;
const TESTING_LIBRARY = /from\s+['"]@testing-library\//;

describe('DOM test convention', () => {
  it('reads the test files', () => {
    expect(Object.keys(tests).length).toBeGreaterThan(300);
    expect(Object.keys(tests)).toContain('./dom.dom.test.ts');
  });

  it('opts into the DOM only through the file name', () => {
    expect(Object.entries(tests).filter(([, text]) => DOCBLOCK.test(text)).map(([path]) => path)).toEqual([]);
  });

  it('uses Testing Library only in *.dom.test.ts', () => {
    const outside = Object.entries(tests)
      .filter(([path, text]) => !path.endsWith('.dom.test.ts') && TESTING_LIBRARY.test(text))
      .map(([path]) => path);
    expect(outside).toEqual([]);
  });
});
