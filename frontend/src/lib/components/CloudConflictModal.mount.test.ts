import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

const TAG = /<CloudConflictModal\b/g;

function markup(source: string): string {
  return source.replace(/<script[\s\S]*?<\/script>/g, '').replace(/<style[\s\S]*?<\/style>/g, '');
}

// Where the `{#if $bigPictureActive …}` switch of App.svelte ends, found by walking the nesting of
// `{#…}` / `{/…}` tags from its start.
function switchEnd(text: string): number {
  const open = text.search(/\{#if \$bigPictureActive\b/);
  expect(open, 'Big Picture switch in App.svelte').toBeGreaterThan(-1);
  const tags = /\{(#|\/)(\w+)[^}]*\}/g;
  tags.lastIndex = open;
  let depth = 0;
  for (let match = tags.exec(text); match; match = tags.exec(text)) {
    depth += match[1] === '#' ? 1 : -1;
    if (depth === 0) return match.index + match[0].length;
  }
  throw new Error('unterminated Big Picture switch in App.svelte');
}

// The `{#…}` blocks still open at a position of the text.
function openBlocks(text: string, end: number): string[] {
  const stack: string[] = [];
  for (const match of text.slice(0, end).matchAll(/\{(#|\/)(\w+)[^}]*\}/g)) {
    if (match[1] === '#') stack.push(match[0]);
    else stack.pop();
  }
  return stack;
}

// A launch in either mode can wait on the conflict dialog, and a background pass opens it in either
// mode: it is mounted once, after the switch, like the emulator prompts, never inside one branch.
// A source tripwire; BigPictureShell.conflict.dom.test.ts drives the dialog over Big Picture.
describe('the save conflict dialog is mounted for both modes', () => {
  const app = markup(frontendSources['App.svelte']);

  it('sits after the Big Picture switch of App.svelte, not inside either branch', () => {
    const at = [...app.matchAll(TAG)].map((match) => match.index ?? -1);
    expect(at).toHaveLength(1);
    expect(at[0]).toBeGreaterThan(switchEnd(app));
    expect(app.search(/<EmulatorNeededModal\b/)).toBeGreaterThan(switchEnd(app));
  });

  it('waits for the sign-in screens to be passed: inside the `linksReady` gate, the condition both branches show the app under', () => {
    const at = app.search(TAG);
    expect(openBlocks(app, at)).toEqual(['{#if linksReady}']);
    const script = frontendSources['App.svelte'].match(/<script[\s\S]*?<\/script>/)?.[0] ?? '';
    const gate = script.match(/const linksReady = \$derived\(([\s\S]*?)\);/)?.[1] ?? '';
    for (const part of ['!$showLegalAcceptance', '!$showTelemetryConsent', "$authState === 'authenticated'", "$authState === 'guest'", "$authState === 'offline'"]) {
      expect(gate, part).toContain(part);
    }
  });

  it('is mounted nowhere else in the frontend', () => {
    const mounts = Object.entries(frontendSources)
      .filter(([path]) => !path.startsWith('lib/testing/'))
      .flatMap(([path, source]) => [...markup(source).matchAll(TAG)].map(() => path));
    expect(mounts).toEqual(['App.svelte']);
  });
});
