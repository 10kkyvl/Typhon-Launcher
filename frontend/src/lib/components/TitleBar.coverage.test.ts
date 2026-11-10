import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

const TAG = /<TitleBar\b/;
const TAGS = /<TitleBar\b/g;

function markup(source: string): string {
  return source.replace(/<script[\s\S]*?<\/script>/g, '').replace(/<style[\s\S]*?<\/style>/g, '');
}

// Splits the `{#if $bigPictureActive …}` block of App.svelte into its Big Picture branch and the
// main-window branch by walking the nesting of `{#…}` / `{:else…}` / `{/…}` tags.
function branches(source: string): { bigPicture: string; main: string } {
  const text = markup(source);
  const open = text.search(/\{#if \$bigPictureActive\b/);
  expect(open, 'Big Picture switch in App.svelte').toBeGreaterThan(-1);
  const tags = /\{(#|:|\/)(\w+)[^}]*\}/g;
  tags.lastIndex = open;
  let depth = 0;
  let elseAt = -1;
  for (let match = tags.exec(text); match; match = tags.exec(text)) {
    const [, kind, word] = match;
    if (kind === '#') depth++;
    else if (kind === '/') depth--;
    else if (kind === ':' && depth === 1 && word === 'else' && elseAt < 0) elseAt = match.index + match[0].length;
    if (depth === 0) {
      expect(elseAt, 'main-window branch').toBeGreaterThan(-1);
      return { bigPicture: text.slice(open, elseAt), main: text.slice(elseAt, match.index) };
    }
  }
  throw new Error('unterminated Big Picture switch in App.svelte');
}

// The open `{#…}` blocks around a position, counted from the start of the given text.
function openBlocks(text: string, end: number): string[] {
  const stack: string[] = [];
  for (const match of text.slice(0, end).matchAll(/\{(#|\/)(\w+)[^}]*\}/g)) {
    if (match[1] === '#') stack.push(match[0]);
    else stack.pop();
  }
  return stack;
}

const SCREEN = /<([A-Z]\w*(?:Screen|Shell))\b/g;

describe('title bar coverage', () => {
  const app = frontendSources['App.svelte'];
  const { bigPicture, main } = branches(app);

  it('is mounted exactly once in the whole frontend, in App.svelte', () => {
    const mounts = Object.entries(frontendSources).flatMap(([path, source]) =>
      [...markup(source).matchAll(TAGS)].map(() => path),
    );
    expect(mounts).toEqual(['App.svelte']);
  });

  it('sits in the main-window branch, outside every condition, before any screen', () => {
    const at = main.search(TAG);
    expect(at).toBeGreaterThan(-1);
    expect(openBlocks(main, at).every((block) => block.startsWith('{#key'))).toBe(true);
    const firstScreen = Math.min(
      ...[...main.matchAll(SCREEN)].map((match) => match.index ?? Infinity),
      main.indexOf('class="boot"'),
    );
    expect(at).toBeLessThan(firstScreen);
  });

  it('puts every full-screen screen of App.svelte under the bar and keeps Big Picture without it', () => {
    const screens = [...markup(app).matchAll(SCREEN)].map((match) => match[1]);
    expect(screens).toEqual(
      expect.arrayContaining(['AppShell', 'AuthScreen', 'LegalAcceptanceScreen', 'TelemetryConsentScreen', 'BigPictureShell']),
    );
    const inMain = [...main.matchAll(SCREEN)].map((match) => match[1]);
    const inBigPicture = [...bigPicture.matchAll(SCREEN)].map((match) => match[1]);
    expect([...inMain, ...inBigPicture].sort()).toEqual([...screens].sort());
    expect(inBigPicture).toEqual(['BigPictureShell']);
    expect(main).toContain('class="boot"');
    expect(bigPicture).not.toMatch(TAG);
  });

  it('reaches every other screen through the shell', () => {
    const elsewhere = Object.entries(frontendSources).flatMap(([path, source]) =>
      [...markup(source).matchAll(SCREEN)]
        .map((match) => match[1])
        .filter(() => path !== 'App.svelte' && !path.startsWith('routes/'))
        .map((name) => `${path}: ${name}`),
    );
    expect(elsewhere).toEqual([]);
  });

  it('never mounts in the overlay window', () => {
    const overlay = Object.entries(frontendSources).filter(([path]) => path.startsWith('overlay/'));
    expect(overlay.length).toBeGreaterThan(0);
    for (const [path, source] of overlay) expect(markup(source), path).not.toMatch(TAG);
    expect(frontendSources['main.ts']).toMatch(/if \(overlay\)[\s\S]*Overlay\.svelte[\s\S]*return;[\s\S]*App\.svelte/);
  });
});
