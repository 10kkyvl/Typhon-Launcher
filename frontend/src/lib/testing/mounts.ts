// What the mock guards of the dialogs read in the sources: an import of a component, the components
// that mount one of a set of dialogs at any depth, and the rule for a test that mounts one only through
// such a component. Each guard globs its own sources (a glob is resolved where it is written) and hands
// them here.

/** An import of one of `names` (`import X from`, `import X, { y } from`, `import(`), in any quote kind. */
export const importOf = (names: string[]) =>
  new RegExp(String.raw`(?:\bimport\b[^'"\x60;]*?\bfrom\s*|\bimport\s*\(\s*)(['"\x60])[^'"\x60]*\/(?:${names.join('|')})\.svelte\1`);

const base = (path: string) => (path.split('/').pop() ?? path).replace(/\.svelte$/, '');

/** The components among `components` (path → source) that mount one of `dialogs`, at any depth, by file name. */
export function wrappers(dialogs: string[], components: Record<string, string>): string[] {
  const found = new Set<string>();
  for (;;) {
    const mounts = importOf([...dialogs, ...found]);
    const more = Object.entries(components)
      .filter(([path, source]) => !dialogs.includes(base(path)) && !found.has(base(path)) && mounts.test(source))
      .map(([path]) => base(path));
    if (more.length === 0) return [...found].sort();
    for (const name of more) found.add(name);
  }
}

type Check = readonly [what: string, holds: (source: string) => boolean];

/** Every mock, unmock or second mock of the Wails runtime. */
export const ANY_RUNTIME = /\bvi\.(?:mock|doMock|unmock|doUnmock)\(\s*['"`]@wailsio\/runtime['"`]/g;
export const REAL_MODULE = (source: string) => !/\bimportActual\b/.test(source) && !/\bvi\.(?:unmock|doUnmock)\(/.test(source);

export const PLAIN = /\(await import\(['"`](?:\.\.?\/)+(?:lib\/)?testing\/dom['"`]\)\)\.wailsRuntime\(\)/;

export const PLAIN_RUNTIME: Check = ['the runtime of lib/testing/dom.ts, once', (source) => PLAIN.test(source) && (source.match(ANY_RUNTIME) ?? []).length === 1];
export const NO_BROWSER_OR_CLIPBOARD: Check = ['no Browser or Clipboard given to it', (source) => !/\b(?:Browser|Clipboard)\b/.test(source)];
export const CALL_IN_PLACE: Check = [
  'a Call only as a vi.fn in place',
  (source) => (source.match(/\bCall\b/g) ?? []).length === (source.match(/\bCall: \{\s*ByID: vi\.fn\(/g) ?? []).length,
];
export const NO_REAL_MODULE: Check = ['no real module behind a mock', REAL_MODULE];

/**
 * A test of a component that mounts a dialog runs on lib/testing/dom.ts's runtime, which has no `Call`,
 * `Browser` or `Clipboard`, so a binding that slips past its mocks throws before anything is sent; it
 * may give it a `Call` only as a `vi.fn` written in place.
 */
export const NEEDED_PLAIN: readonly Check[] = [PLAIN_RUNTIME, NO_BROWSER_OR_CLIPBOARD, CALL_IN_PLACE, NO_REAL_MODULE];

export const gaps = (needed: readonly Check[], source: string) => needed.filter(([, holds]) => !holds(source)).map(([what]) => what);
