import { describe, expect, it } from 'vitest';

/**
 * Every typeface is a theme token (`--font-sans`, `--font-mono` in tokens.css), so a theme can change
 * it and the bundled faces are the only ones the UI asks for. A component never names a typeface: it
 * takes `var(--font-sans)` / `var(--font-mono)` without a fallback list, or inherits.
 */

// Read from disk: vitest serves `.css?raw` imports as empty strings, and the frontend has no Node typings.
type Fs = {
  readFileSync(path: URL, encoding: 'utf8'): string;
  readdirSync(path: URL, options: { recursive: true }): string[];
  existsSync(path: URL): boolean;
};
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');

const SRC = new URL('../', import.meta.url);
const NODE_MODULES = new URL('../../node_modules/', import.meta.url);
const TOKENS = 'styles/tokens.css';
const SELF = 'styles/font-tokens.test.ts';

const read = (path: string) => fs.readFileSync(new URL(path, SRC), 'utf8');
const files = fs
  .readdirSync(SRC, { recursive: true })
  .map((path) => path.replaceAll('\\', '/'))
  .filter((path) => /\.(ts|js|svelte|css|html|json|md)$/.test(path) && path !== SELF);
const styled = files.filter((path) => /\.(svelte|css)$/.test(path) && path !== TOKENS);

const FAMILY = /font-family\s*:\s*([^;}\n]+)/g;
const ALLOWED = /^(?:inherit|initial|unset|var\(--font-(?:sans|mono)\))$/;

function literals(text: string): string[] {
  const bad: string[] = [];
  for (const m of text.matchAll(FAMILY)) {
    const value = m[1].trim().replace(/\s*!important$/, '');
    if (!ALLOWED.test(value)) bad.push(value);
  }
  return bad;
}

function tokenFamilies(name: string): string[] {
  const value = new RegExp(`${name}\\s*:\\s*([^;]+);`).exec(read(TOKENS))?.[1];
  expect(value, `${name} in tokens.css`).toBeTypeOf('string');
  return (value as string).split(',').map((f) => f.trim().replace(/^['"]|['"]$/g, ''));
}

function bundledFaces(): Set<string> {
  const faces = new Set<string>();
  for (const m of read('main.ts').matchAll(/^import\s+'(@fontsource[^']*)';$/gm)) {
    const entry = new URL(m[1].endsWith('.css') ? m[1] : `${m[1]}/index.css`, NODE_MODULES);
    expect(fs.existsSync(entry), `${m[1]} resolves to ${entry.pathname}`).toBe(true);
    for (const f of fs.readFileSync(entry, 'utf8').matchAll(/font-family\s*:\s*['"]([^'"]+)['"]/g)) faces.add(f[1]);
  }
  return faces;
}

describe('font tokens', () => {
  it('has files to scan', () => {
    expect(styled.length).toBeGreaterThan(50);
    expect(read(TOKENS)).toContain('--font-sans');
  });

  it('names no typeface outside tokens.css: var(--font-sans), var(--font-mono) or inherit only', () => {
    const offenders = styled.flatMap((path) => literals(read(path)).map((v) => `${path}: font-family: ${v}`));
    expect(offenders).toEqual([]);
  });

  it('keeps no trace of the previous interface font', () => {
    const offenders = files.filter((path) => /inter[\s-]tight/i.test(read(path)));
    expect(offenders).toEqual([]);
  });

  it('opens each font token with a face the bundle provides', () => {
    const faces = bundledFaces();
    for (const token of ['--font-sans', '--font-mono']) {
      expect(faces, `${token} first family`).toContain(tokenFamilies(token)[0]);
    }
  });
});
