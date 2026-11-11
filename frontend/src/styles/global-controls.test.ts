import { describe, expect, it } from 'vitest';
import { frontendSources } from '../lib/testing/sources';

/**
 * Rules of global.css that every screen relies on and no DOM test can see (jsdom applies no
 * stylesheet): they are read from the source. Limit: a rule moved to another file, or overridden
 * from a component, is not seen here.
 */

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const css = fs.readFileSync(new URL('./global.css', import.meta.url), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');

function body(selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  return new RegExp(`(?:^|\\})\\s*${escaped}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? '';
}

describe('a text field', () => {
  // The field's border takes the focus colour; the outline every control gets on keyboard focus is
  // drawn over that border, not as a second ring around it.
  it('shows one focus ring', () => {
    expect(body('.input')).not.toMatch(/outline\s*:/);
    expect(body('.input:focus-visible')).toMatch(/outline: 2px solid var\(--accent-ring\);/);
    expect(body('.input:focus-visible')).toMatch(/outline-offset: -1px;/);
  });
});

describe('selecting text', () => {
  const list = /\n((?:[^\n{}]+,\n)+[^\n{}]+) \{\s*user-select: none;\s*\}/.exec(css)?.[1].split(',').map((selector) => selector.trim()) ?? [];

  // Content can be selected and copied (an error, a path, a description); the window's furniture cannot.
  it('is allowed by default and switched off for controls, navigation and the window bars, in one rule', () => {
    expect(body('body')).not.toMatch(/user-select/);
    for (const selector of ['button', 'a', 'label', "[role='tab']", "[role='switch']", "[role='menu']", "[data-part='sidebar']", "[data-part='topbar']", '.titlebar']) {
      expect(list, selector).toContain(selector);
    }
  });

  it('stays off everywhere in Big Picture and the overlay window, as before', () => {
    expect(list).toContain('.big-picture');
    expect(list).toContain('html.overlay-window body');
  });

  it('is always possible in a text field, whatever it sits in', () => {
    expect(css).toMatch(/\ninput,\ntextarea \{\s*user-select: text;\s*\}/);
  });

  // Text that must stay selectable inside a control says so itself; nothing else needs to.
  it('is switched back on only inside controls, and in the overlay chat', () => {
    const KEPT = ['lib/components/InstallModal.svelte', 'overlay/OverlayChat.svelte', 'routes/friends/FriendCodeCard.svelte'];
    const found = Object.entries(frontendSources)
      .filter(([path, text]) => path.endsWith('.svelte') && /user-select:\s*text/.test(text))
      .map(([path]) => path)
      .sort();
    expect(found).toEqual(KEPT);
  });
});
