import { describe, expect, it } from 'vitest';

/**
 * The dialog components keep a visible focus ring and paint only with defined tokens. The image viewer
 * lost its ring to `var(--text-1)`, a name no stylesheet defines: the outline fell back to nothing.
 *
 * - An `outline: none` (or `0`) comes with a replacement focus style in the same rule.
 * - Every `var(--name)` they use is defined in tokens.css or shell.css, or by one of them (the viewer sets
 *   `--zoom-progress` for the slider stylesheet it imports).
 */

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const SRC = new URL('../../', import.meta.url);
const read = (path: string) => fs.readFileSync(new URL(path, SRC), 'utf8');

const DIALOGS = ['lib/components/Modal.svelte', 'lib/components/ConfirmModal.svelte', 'lib/components/Lightbox.svelte', 'styles/zoom-slider.css'];
const RULE = /([^{}]*)\{([^{}]*)\}/g;
const NO_OUTLINE = /(?<![-\w])outline\s*:\s*(?:none|0)\s*(?:;|$)/;
const REPLACEMENT = /(?<![-\w])(?:box-shadow|border-color|outline-color|background(?:-color)?)\s*:\s*(?!none)/;

function definitions(text: string): Set<string> {
  return new Set([...text.matchAll(/(?:^|[\s;{"'])(--[\w-]+)\s*:/gm), ...text.matchAll(/style:(--[\w-]+)/g)].map((m) => m[1]));
}

const defined = new Set(['styles/tokens.css', 'styles/shell.css', ...DIALOGS].flatMap((path) => [...definitions(read(path))]));

describe('dialog styles', () => {
  it('never drop the focus ring without a replacement', () => {
    const bare = DIALOGS.flatMap((path) =>
      [...read(path).matchAll(RULE)]
        .filter((m) => NO_OUTLINE.test(m[2]) && !REPLACEMENT.test(m[2]))
        .map((m) => `${path} ${m[1].trim().split('\n').pop()}`),
    );
    expect(bare).toEqual([]);
  });

  it('use only defined custom properties', () => {
    const undefinedNames = DIALOGS.flatMap((path) => {
      const used = [...read(path).matchAll(/var\((--[\w-]+)/g)].map((m) => m[1]);
      return used.filter((name) => !defined.has(name)).map((name) => `${path} ${name}`);
    });
    expect(undefinedNames).toEqual([]);
  });
});
