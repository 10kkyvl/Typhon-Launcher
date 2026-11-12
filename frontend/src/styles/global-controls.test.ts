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

// The selectors of a list, split on the commas outside parentheses (`:is(a, b)` is one selector).
function selectors(list: string): string[] {
  const out = [''];
  let depth = 0;
  for (const ch of list) {
    if (ch === '(') depth++;
    if (ch === ')') depth--;
    if (ch === ',' && depth === 0) out.push('');
    else out[out.length - 1] += ch;
  }
  return out.map((part) => part.trim());
}

// Every rule whose selector list holds this selector, as written.
function rules(selector: string): string[] {
  return [...css.matchAll(/([^{}]+)\{([^{}]*)\}/g)].filter((m) => selectors(m[1]).includes(selector)).map((m) => m[2]);
}

// Every stylesheet, and every component's <style> block, by path. Vitest serves a .css import empty,
// so the glob only lists the files and they are read from disk.
const stylesheets: Record<string, string> = {
  ...Object.fromEntries(
    Object.keys(import.meta.glob('../**/*.css')).map((path) => [
      path.replace(/^\.\.\//, '').replace(/^\.\//, 'styles/'),
      fs.readFileSync(new URL(path, import.meta.url), 'utf8'),
    ]),
  ),
  ...Object.fromEntries(
    Object.entries(frontendSources)
      .filter(([path]) => path.endsWith('.svelte'))
      .map(([path, text]) => [path, [...text.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n')]),
  ),
};

// The selector lists of every rule in a stylesheet.
function selectorLists(text: string): string[][] {
  return [...text.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}]+)\{[^{}]*\}/g)].map((m) => selectors(m[1]));
}

describe('a selector an engine may not know', () => {
  // A selector list is all or nothing: a WebView without :has() (WebKit before 15.4; macOS 12 is
  // allowed) drops the whole rule, and every other selector in the list with it.
  it('sits in a rule of its own: :has() never shares a list', () => {
    expect(stylesheets['styles/global.css']).toContain('user-select');
    expect(selectorLists('a,\nb:has(.x) {}\nc:has(.y) {}')).toEqual([['a', 'b:has(.x)'], ['c:has(.y)']]);
    const shared = Object.entries(stylesheets).flatMap(([path, text]) =>
      selectorLists(text)
        .filter((list) => list.length > 1 && list.some((selector) => selector.includes(':has(')))
        .map((list) => `${path}: ${list.join(', ')}`),
    );
    expect(shared).toEqual([]);
  });
});

// The look Big Picture and the overlay window were drawn with (redesign-2.0's global.css).
const PINNED_SCOPES = ['.big-picture', 'html:has(.big-picture)', 'html.overlay-window'];

describe('the scrollbar', () => {
  it('is drawn from the theme in the main window', () => {
    expect(body('::-webkit-scrollbar-thumb')).toMatch(/background: color-mix\(in srgb, var\(--text-3\) \d+%, transparent\);/);
  });

  it('keeps the thumb Big Picture and the overlay window had, with the dialogs Big Picture opens', () => {
    for (const scope of PINNED_SCOPES) {
      const thumb = rules(`${scope} ::-webkit-scrollbar-thumb`).join('');
      expect(thumb, scope).toMatch(/background: rgba\(255, 255, 255, 0\.09\);/);
      expect(thumb, scope).toMatch(/border-radius: 0\.8rem;/);
      expect(rules(`${scope} ::-webkit-scrollbar-thumb:hover`).join(''), `${scope} hover`).toMatch(/background: rgba\(255, 255, 255, 0\.16\);/);
    }
  });
});

describe('a text field', () => {
  // The field's border takes the focus colour; the outline every control gets on keyboard focus is
  // drawn over that border, not as a second ring around it.
  it('shows one focus ring', () => {
    expect(body('.input')).not.toMatch(/outline\s*:/);
    expect(body('.input:focus-visible')).toMatch(/outline: 2px solid var\(--accent-ring\);/);
    expect(body('.input:focus-visible')).toMatch(/outline-offset: -1px;/);
  });

  // The ring is drawn over the border, so an invalid field would lose its red edge while focused.
  it('keeps saying it holds a bad value while focused: the ring takes the danger colour', () => {
    expect(body(".input[aria-invalid='true']:focus-visible")).toMatch(/outline-color: var\(--danger\);/);
  });

  // A locked field takes no input: it must not look like one that does, with the strong edge, a
  // hover that strengthens it, or the text cursor. Its look comes from tokens.
  it('looks locked while disabled, and does not answer the pointer', () => {
    expect(body('.input:hover:not(:disabled)')).toMatch(/border-color: var\(--text-3\);/);
    expect(body('.input:hover')).toBe('');
    const locked = body('.input:disabled');
    expect(locked).toMatch(/border-color: var\(--border\);/);
    expect(locked).toMatch(/background: var\(--hover\);/);
    expect(locked).toMatch(/color: var\(--text-3\);/);
    expect(locked).toMatch(/cursor: default;/);
    expect(locked).not.toMatch(/#[0-9a-f]{3,8}\b|rgba?\(/i);
  });

  // These forms mark a bad value with aria-invalid and leave focus to the global ring: an own
  // `outline: none` would hide it again, leaving only a halo too faint to see on an invalid field.
  it('shows the global ring on the sign-in and support forms', () => {
    for (const path of ['routes/auth/AuthScreen.svelte', 'routes/support/NewTicket.svelte', 'routes/support/TicketView.svelte']) {
      const style = /<style>([\s\S]*?)<\/style>/.exec(frontendSources[path] ?? '')?.[1] ?? '';
      expect(style, path).toMatch(/\.input \{/);
      expect(style.match(/\.input:focus[^{]*\{[^}]*outline:\s*none/g), path).toBeNull();
    }
  });
});

// The opening tags of the error and failure messages inside every <label> of a component.
function labelMessages(text: string): string[] {
  return [...text.matchAll(/<label\b[\s\S]*?<\/label>/g)].flatMap((label) =>
    [...label[0].matchAll(/<\w+\b[^<>]*\bclass="(?:error|failure)\b[^"]*"[^<>]*>/g)].map((tag) => tag[0]),
  );
}

describe('selecting text', () => {
  const list = /\n((?:[^\n{}]+,\n)+[^\n{}]+) \{\s*user-select: none;\s*\}/.exec(css)?.[1].split(',').map((selector) => selector.trim()) ?? [];

  // Content can be selected and copied (an error, a path, a description); the window's furniture cannot.
  it('is allowed by default and switched off for controls, navigation and the window bars, in one rule', () => {
    expect(body('body')).not.toMatch(/user-select/);
    for (const selector of ['button', 'a', 'label', "[role='tab']", "[role='switch']", "[role='menu']", "[data-part='sidebar']", "[data-part='topbar']", '.titlebar']) {
      expect(list, selector).toContain(selector);
    }
  });

  // Shared dialogs over Big Picture are portalled to the end of <body>, outside `.big-picture`: the
  // document is in Big Picture while the shell is in it, so the rule is set on <body> from there.
  it('stays off everywhere in Big Picture, the dialogs it opens included, and in the overlay window, as before', () => {
    expect(list).toContain('.big-picture');
    expect(list).toContain('html.overlay-window body');
    expect(rules('html:has(.big-picture) body').join('')).toMatch(/user-select: none;/);
  });

  // An error inside a <label> (the label is furniture) must still be copied: a message says what it is
  // with its role, and the role makes it selectable, in the main window only.
  it('is possible in a message wherever it sits, by its role, except in Big Picture and the overlay window', () => {
    const rule = rules("html:not(.overlay-window):not(:has(.big-picture)) :is([role='alert'], [role='status'])").join('');
    expect(rule).toMatch(/user-select: text;/);
  });

  it('finds every error message inside a label saying what it is', () => {
    const fixture = '<label><input /><span class="error" role="alert">Bad</span></label><span class="error">Out</span><label><p class="failure x">No</p></label>';
    expect(labelMessages(fixture)).toEqual(['<span class="error" role="alert">', '<p class="failure x">']);
    const messages = Object.entries(frontendSources)
      .filter(([path]) => path.endsWith('.svelte'))
      .flatMap(([path, text]) => labelMessages(text).map((tag) => `${path}: ${tag}`));
    // Some forms show their errors inside the label, so a scan that finds none has stopped reading them.
    expect(messages).not.toEqual([]);
    expect(messages.filter((tag) => !/\brole="(?:alert|status)"/.test(tag))).toEqual([]);
  });

  it('is always possible in a text field, whatever it sits in', () => {
    expect(css).toMatch(/\ninput,\ntextarea \{\s*user-select: text;\s*\}/);
  });

  // Text that must stay selectable inside a control says so itself; nothing else needs to. In the
  // stylesheets only global.css switches it on: for text fields and for messages.
  it('is switched back on only inside controls, and in the overlay chat', () => {
    const KEPT = ['lib/components/InstallModal.svelte', 'overlay/OverlayChat.svelte', 'routes/friends/FriendCodeCard.svelte'];
    const found = Object.entries(frontendSources)
      .filter(([path, text]) => path.endsWith('.svelte') && /user-select:\s*text/.test(text))
      .map(([path]) => path)
      .sort();
    expect(found).toEqual(KEPT);

    const GLOBAL = ['styles/global.css: input, textarea', "styles/global.css: html:not(.overlay-window):not(:has(.big-picture)) :is([role='alert'], [role='status'])"];
    const sheets = Object.entries(stylesheets)
      .filter(([path]) => path.endsWith('.css'))
      .flatMap(([path, text]) =>
        [...text.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}]+)\{([^{}]*)\}/g)]
          .filter((m) => /user-select:\s*text/.test(m[2]))
          .map((m) => `${path}: ${selectors(m[1]).join(', ')}`),
      );
    expect(sheets).toEqual(GLOBAL);
  });
});
