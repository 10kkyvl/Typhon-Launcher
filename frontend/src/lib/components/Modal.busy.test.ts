import { describe, expect, it } from 'vitest';

/**
 * A dialog that disables its buttons while an operation runs is not closable by Escape, its close
 * button or the scrim either: closing it mid-operation writes the result into a destroyed component
 * (the error is lost) or reads as "cancelled" when nothing was. So a `<Modal>` whose footer has a
 * `disabled={…}` naming an operation flag (`busy`, `pending`, or a lowercase `…ing` word such as
 * `saving`) passes that flag to `dismissible`. A dialog that stays closable on purpose (the operation
 * goes on in the background and is reported elsewhere, or closing only drops a read) says so in a
 * comment right before the tag: `<!-- dismissible while <flag>: <why> -->`.
 *
 * Limits: only the footer snippet is read (a busy flag on a button in the body is not seen); a flag
 * reached through a store (`$busy`) or a derived name that does not end in -ing is not seen; the check
 * is that the flag's name appears in `dismissible={…}`, not that the expression is right.
 */

const sources = import.meta.glob<string>(['../../**/*.svelte'], { query: '?raw', import: 'default', eager: true });

const FLAG = /(?<![\w$.])(busy|pending|[a-z]+ing)(?![\w$])/g;

function closableWhileBusy(): string[] {
  const found: string[] = [];
  for (const [path, text] of Object.entries(sources)) {
    if (path.includes('/testing/')) continue;
    for (const tag of text.matchAll(/<Modal\b/g)) {
      const start = tag.index ?? 0;
      const block = text.slice(start, text.indexOf('</Modal>', start));
      const head = block.slice(0, block.search(/>\s*\n/) + 1);
      const footer = /\{#snippet footer\(\)\}([\s\S]*?)\{\/snippet\}/.exec(block)?.[1] ?? '';
      const flags = new Set([...footer.matchAll(/disabled=\{([^}]*)\}/g)].flatMap((m) => [...m[1].matchAll(FLAG)].map((f) => f[1])));
      const dismissible = /dismissible=\{([^}]*)\}/.exec(head)?.[1] ?? '';
      const before = text.slice(Math.max(0, start - 400), start);
      for (const flag of flags) {
        if (new RegExp(`(?<![\\w$])${flag}(?![\\w$])`).test(dismissible)) continue;
        if (new RegExp(`<!-- dismissible while ${flag}: [^>]+-->\\s*$`).test(before)) continue;
        found.push(`${path}:${text.slice(0, start).split('\n').length} ${flag}`);
      }
    }
  }
  return found;
}

describe('busy dialogs', () => {
  it('reads the dialogs', () => {
    expect(Object.values(sources).filter((text) => text.includes('<Modal')).length).toBeGreaterThan(30);
  });

  it('cannot be dismissed while the operation that disables their buttons runs', () => {
    expect(closableWhileBusy()).toEqual([]);
  });
});
