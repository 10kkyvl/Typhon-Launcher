import { describe, expect, it } from 'vitest';
import { goSources } from '../lib/testing/sources';

/**
 * Every typeface is a theme token (`--font-sans`, `--font-mono` in tokens.css), so a theme can change
 * it and the bundled faces are the only ones the UI asks for. A component never names a typeface: it
 * takes `var(--font-sans)` / `var(--font-mono)` without a fallback list, or inherits. The one exception
 * is the brand name: `var(--font-wordmark)` (shell.css, not themable) in the rules listed in
 * WORDMARK_SITES and nowhere else.
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
const SHELL = 'styles/shell.css';
const SELF = 'styles/font-tokens.test.ts';

const read = (path: string) => fs.readFileSync(new URL(path, SRC), 'utf8');
const files = fs
  .readdirSync(SRC, { recursive: true })
  .map((path) => path.replaceAll('\\', '/'))
  .filter((path) => /\.(ts|js|svelte|css|html|json|md)$/.test(path) && path !== SELF);
const styled = files.filter((path) => /\.(svelte|css)$/.test(path) && path !== TOKENS);

const FAMILY = /(?:font-family\s*:|style:font-family=)\s*([^;}\n]+)/g;
const SHORTHAND = /(?<![-\w])font\s*:\s*([^;}\n]+)/g;
const MONO_RULE = /\{[^{}]*font-family\s*:\s*var\(--font-mono\)[^{}]*\}/g;
const ALLOWED = /^(?:inherit|initial|unset|var\(--font-(?:sans|mono)\))$/;
const WORDMARK = 'var(--font-wordmark)';
const WORDMARK_RULE = /([^{}]*)\{[^{}]*font-family\s*:\s*var\(--font-wordmark\)[^{}]*\}/g;
const WORDMARK_SITES = ['lib/components/Sidebar.svelte .wordmark'];

function literals(text: string): string[] {
  const bad: string[] = [];
  for (const m of text.matchAll(FAMILY)) {
    const value = m[1].trim().replace(/\s*!important$/, '');
    if (!ALLOWED.test(value) && value !== WORDMARK) bad.push(value);
  }
  return bad;
}

// Every use of the wordmark face, as `path selector`; one written outside a rule block shows as `path ?`.
function wordmarkSites(path: string, text: string): string[] {
  const inRules = [...text.matchAll(WORDMARK_RULE)].map((m) => `${path} ${m[1].trim().split('\n').pop()?.trim()}`);
  const all = [...text.matchAll(FAMILY)].filter((m) => m[1].trim().replace(/\s*!important$/, '') === WORDMARK).length;
  return all === inRules.length ? inRules : [...inRules, `${path} ?`];
}

function tokenFamilies(name: string, file = TOKENS): string[] {
  const value = new RegExp(`${name}\\s*:\\s*([^;]+);`).exec(read(file))?.[1];
  expect(value, `${name} in ${file}`).toBeTypeOf('string');
  return (value as string).split(',').map((f) => f.trim().replace(/^['"]|['"]$/g, ''));
}

function presetFamilies(name: string): string[] {
  const presets = goSources['internal/theme/presets.go'] ?? '';
  const first = [...presets.matchAll(new RegExp(`"${name}":\\s*"([^",]+)`, 'g'))].map((m) => m[1].trim());
  expect(first.length, `${name} in presets.go`).toBeGreaterThanOrEqual(3);
  return first;
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

  it('sets a typeface through the font shorthand nowhere', () => {
    const offenders = styled.flatMap((path) =>
      [...read(path).matchAll(SHORTHAND)].map((m) => m[1].trim()).filter((v) => v !== 'inherit').map((v) => `${path}: font: ${v}`),
    );
    expect(offenders).toEqual([]);
  });

  // JetBrains Mono joins "www", "//", "!=" and "->" into single glyphs; text shown as "exactly what is
  // sent", a path or a licence has to read character by character.
  it('turns ligatures off wherever the monospace font is used', () => {
    const offenders = styled.flatMap((path) =>
      [...read(path).matchAll(MONO_RULE)].filter((m) => !/font-variant-ligatures\s*:\s*none/.test(m[0])).map(() => path),
    );
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
      for (const family of presetFamilies(token)) expect(faces, `${token} first family in presets.go`).toContain(family);
    }
  });

  it('opens the wordmark token with a face the bundle provides', () => {
    expect(bundledFaces(), '--font-wordmark first family').toContain(tokenFamilies('--font-wordmark', SHELL)[0]);
  });

  // The subset files declare no unicode-range, so the face declared last is tried first for every
  // character: latin goes last, or the brand name pays for a cyrillic fetch it does not use.
  it('bundles the wordmark weights in use, cyrillic before latin for each', () => {
    const imports = [...read('main.ts').matchAll(/^import\s+'@fontsource\/unbounded\/([^']+)\.css';$/gm)].map((m) => m[1]);
    const weights = new Set(
      WORDMARK_SITES.map((site) => {
        const [path, selector] = site.split(' ');
        const body = new RegExp(`\\n\\s*${selector.replace('.', '\\.')} \\{([^}]*)\\}`).exec(read(path))?.[1] ?? '';
        return /font-weight:\s*(\d+)/.exec(body)?.[1] ?? '400';
      }),
    );
    expect(imports).toEqual([...weights].flatMap((w) => [`cyrillic-${w}`, `latin-${w}`]));
  });

  it('sets the wordmark face on the brand name only', () => {
    const sites = styled.flatMap((path) => wordmarkSites(path, read(path)));
    expect(sites.filter((site) => !WORDMARK_SITES.includes(site))).toEqual([]);
  });
});
