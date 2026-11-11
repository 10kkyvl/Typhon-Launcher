import { describe, expect, it } from 'vitest';

/**
 * One portal. Only lib/utils/portal.ts moves a rendered node out of its place in the tree: no
 * component appends to `document.body` or `document.documentElement` itself, and no other module
 * defines something named `portal`. Plain TS that creates its own detached host (the theme guard,
 * the colour probe) is not a portal and is not matched: it never touches a Svelte-owned node.
 */

const sources = import.meta.glob<string>(['../../**/*.{ts,svelte}', '!../../**/*.test.ts'], {
  query: '?raw',
  import: 'default',
  eager: true,
});

const OWNER = './portal.ts';
const MOVES = /document\.(?:body|documentElement)\.(?:appendChild|append|prepend|insertBefore|replaceChildren)\s*\(/;
const DEFINES = /(?:function\s+portal\b|(?:const|let|var)\s+portal\b\s*[:=])/;

describe('portal', () => {
  it('reads the sources', () => {
    expect(Object.keys(sources).length).toBeGreaterThan(300);
    expect(Object.keys(sources)).toContain(OWNER);
  });

  it('is defined once', () => {
    const defining = Object.entries(sources).filter(([, text]) => DEFINES.test(text)).map(([path]) => path);
    expect(defining).toEqual([OWNER]);
  });

  it('is the only way a component moves its nodes to the document', () => {
    const moving = Object.entries(sources)
      .filter(([path, text]) => path.endsWith('.svelte') && MOVES.test(text))
      .map(([path]) => path);
    expect(moving).toEqual([]);
  });
});
