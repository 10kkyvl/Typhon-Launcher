import { describe, expect, it } from 'vitest';
import { frontendSources } from '../lib/testing/sources';

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');

// Big Picture and the overlay window have no title bar: their full-viewport sizes stay whole.
const outsideMainWindow = (path: string) =>
  path.startsWith('lib/bigpicture/') || path.startsWith('overlay/') || path === 'lib/components/BigPictureShell.svelte';

const sheets: [string, string][] = [
  ...Object.entries(frontendSources).filter(([path]) => path.endsWith('.svelte') && !outsideMainWindow(path)),
  ...['global.css', 'shell.css'].map((name): [string, string] => [`styles/${name}`, fs.readFileSync(new URL(name, import.meta.url), 'utf8')]),
];

function declarations(pattern: RegExp): string[] {
  return sheets.flatMap(([path, text]) =>
    [...text.matchAll(pattern)].map((match) => `${path}:${text.slice(0, match.index).split('\n').length} ${match[0].trim()}`),
  );
}

describe('main window viewport height', () => {
  it('has main-window sources to scan', () => {
    expect(sheets.length).toBeGreaterThan(100);
  });

  it('sizes no screen to the bare viewport height under the title bar', () => {
    expect(declarations(/(?:^|[\s;{])(?:min-|max-)?height:\s*100[ds]?vh\s*(?:;|$)/gm)).toEqual([]);
  });

  it('subtracts the title bar wherever it subtracts the top bar from the viewport', () => {
    const bad = declarations(/[-\w]+:\s*[^;{}]*100[ds]?vh[^;{}]*var\(--topbar-h\)[^;{}]*/g).filter(
      (line) => !line.includes('var(--titlebar-h)'),
    );
    expect(bad).toEqual([]);
  });
});
