import { describe, expect, it } from 'vitest';
import { frontendSources } from '../lib/testing/sources';

/**
 * The status components (progress bar, badges, empty state, page header, card) take every colour,
 * radius and duration from tokens, are not restyled from outside, and the progress bar gets its
 * colour as a tone and always has a name. These checks read source text: they catch the habit coming
 * back, the behaviour itself is held by the components' DOM tests.
 */

const COMPONENTS = ['ProgressBar', 'PageHeader', 'Card', 'StatusBadge', 'SourceBadge', 'EmptyState'];
const path = (name: string) => `lib/components/${name}.svelte`;
const styleOf = (source: string) => /<style[^>]*>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '';
const rule = (css: string, selector: string) =>
  new RegExp(`(?:^|\\})\\s*${selector.replace(/[.[\]'=]/g, '\\$&')}\\s*\\{([^}]*)\\}`).exec(css.replace(/\/\*[\s\S]*?\*\//g, ''))?.[1] ?? '';
const value = (body: string, property: string) => new RegExp(`(?:^|;|\\s)${property}:\\s*([^;]+);`).exec(body)?.[1].trim();

function literals(css: string): string[] {
  const found: string[] = [];
  const bare = css.replace(/\/\*[\s\S]*?\*\//g, '');
  for (const match of bare.matchAll(/#[0-9a-f]{3,8}\b|\b(?:rgba?|hsla?)\(/gi)) found.push(`colour ${match[0]}`);
  for (const match of bare.matchAll(/(?<![\w-])\d*\.?\d+m?s\b/g)) found.push(`duration ${match[0]}`);
  for (const match of bare.matchAll(/border-radius:\s*([^;]+);/g)) {
    if (/\d/.test(match[1].replace(/var\([^)]*\)/g, ''))) found.push(`radius ${match[1].trim()}`);
  }
  return found;
}

/** Index of the `>` that closes a tag whose name ends at `from`: a `>` inside `{…}` or a quoted value is part of an attribute. */
function tagEnd(source: string, from: number): number {
  let depth = 0;
  let quote = '';
  for (let i = from; i < source.length; i++) {
    const char = source[i];
    if (quote) {
      if (char === quote) quote = '';
    } else if (char === '{') depth++;
    else if (char === '}') depth--;
    else if (char === '"' || char === "'" || (depth > 0 && char === '`')) quote = char;
    else if (char === '>' && depth === 0) return i;
  }
  return -1;
}

/** Opening tags of a component, read to their closing `>`. */
function tags(source: string, name: string): string[] {
  return [...source.matchAll(new RegExp(`<${name}\\b`, 'g'))]
    .map((match) => ({ start: match.index, end: tagEnd(source, match.index + match[0].length) }))
    .filter(({ end }) => end >= 0)
    .map(({ start, end }) => source.slice(start, end + 1));
}

const VOID = new Set(['area', 'br', 'col', 'embed', 'hr', 'img', 'input', 'link', 'meta', 'source', 'track', 'wbr']);
const CONTROL = /^<(?:Button|IconButton|button|a)\b|\brole="(?:button|link|menuitem|tab|option|switch|checkbox)"/;

/** Every progress bar tag of a component's markup, and whether an element that is a control encloses it. */
function barsInMarkup(source: string): { tag: string; inControl: boolean }[] {
  const markup = source.replace(/<script[\s\S]*?<\/script>|<style[\s\S]*?<\/style>|<!--[\s\S]*?-->/g, (block) => ' '.repeat(block.length));
  const open: { name: string; control: boolean }[] = [];
  const bars: { tag: string; inControl: boolean }[] = [];
  const pattern = /<(\/?)([A-Za-z][\w.:-]*)/g;
  for (let match = pattern.exec(markup); match; match = pattern.exec(markup)) {
    const end = tagEnd(markup, match.index + match[0].length);
    if (end < 0) break;
    pattern.lastIndex = end + 1;
    const [, closing, name] = match;
    const tag = markup.slice(match.index, end + 1);
    if (closing) {
      const at = open.map((element) => element.name).lastIndexOf(name);
      if (at >= 0) open.length = at;
      continue;
    }
    if (name === 'ProgressBar') bars.push({ tag, inControl: open.some((element) => element.control) });
    if (tag.endsWith('/>') || VOID.has(name)) continue;
    open.push({ name, control: CONTROL.test(tag) });
  }
  return bars;
}

describe('status components take their look from tokens', () => {
  it('finds a literal when there is one', () => {
    expect(literals('.a { color: #fff; transition: width 600ms linear; border-radius: 99rem; }')).toEqual([
      'colour #fff',
      'duration 600ms',
      'radius 99rem',
    ]);
    expect(literals('.a { border-radius: var(--radius-bar); transition: transform var(--progress-step) linear; }')).toEqual([]);
  });

  for (const name of COMPONENTS) {
    it(`${name} writes no colour, radius or duration of its own`, () => {
      const css = styleOf(frontendSources[path(name)] ?? '');
      expect(css, `${name} has a style block`).not.toBe('');
      expect(literals(css)).toEqual([]);
    });
  }
});

describe('status components are not restyled from outside', () => {
  const REACH = /:global\(\s*\.(track|page-header|card|badge|empty)\b/;

  it('no other component reaches into them through :global', () => {
    const owners = new Set(COMPONENTS.map(path));
    const offenders = Object.entries(frontendSources)
      .filter(([file, source]) => file.endsWith('.svelte') && !owners.has(file) && REACH.test(source))
      .map(([file]) => file);
    expect(offenders).toEqual([]);
  });
});

describe('every progress bar', () => {
  const uses = Object.entries(frontendSources)
    .filter(([file]) => file.endsWith('.svelte'))
    .flatMap(([file, source]) => tags(source, 'ProgressBar').map((tag) => ({ file, tag })));

  it('is read in full wherever the app draws one', () => {
    const written = Object.entries(frontendSources)
      .filter(([file]) => file.endsWith('.svelte'))
      .reduce((sum, [, source]) => sum + [...source.matchAll(/<ProgressBar\b/g)].length, 0);
    expect(uses.length).toBe(written);
    expect(uses.length).toBeGreaterThan(20);
  });

  it('reads a tag the way the rules below need', () => {
    expect(tags('<ProgressBar value={a > 0 ? 1 : 0} color="red" />', 'ProgressBar')).toEqual(['<ProgressBar value={a > 0 ? 1 : 0} color="red" />']);
    expect(tags('<ProgressBar value={1} color="red" />', 'ProgressBar')).toHaveLength(1);
    expect(/\bcolor=/.test('<ProgressBar value={1} color="red" />')).toBe(true);
    expect(/\b(label|labelledby)=/.test('<ProgressBar value={1} />')).toBe(false);
  });

  it('knows when a control encloses it', () => {
    const markup =
      '<script>const a = 1;</script><Button onclick={() => go(a > 0)}><span><ProgressBar value={1} decorative /></span></Button>' +
      '<div><input /><ProgressBar value={2} label="x" /></div><div role="button"><ProgressBar value={3} /></div>';
    expect(barsInMarkup(markup).map(({ inControl }) => inControl)).toEqual([true, false, true]);
  });

  // A control's content is its name: a bar in it adds its value to that name, and the control already says the progress.
  it('is decorative exactly when it sits inside a control', () => {
    const offenders = Object.entries(frontendSources)
      .filter(([file]) => file.endsWith('.svelte'))
      .flatMap(([file, source]) => barsInMarkup(source).map((bar) => ({ file, ...bar })))
      .filter(({ tag, inControl }) => inControl !== /\bdecorative\b/.test(tag))
      .map(({ file, tag, inControl }) => `${file}: ${inControl ? 'in a control' : 'decorative outside a control'}: ${tag.slice(0, 50)}`);
    expect(offenders).toEqual([]);
  });

  // A tripwire for the usual ways a name picks up the amount: a percent, a progress or summary line, a stage.
  it('is named for what it measures, the amount left to its value', () => {
    const offenders = uses
      .filter(({ tag }) => /\blabel=\{[^}]*(?:[Pp]ercent|Progress|Summary|\.text\b|\bstage)/.test(tag))
      .map(({ file, tag }) => `${file}: ${tag.slice(0, 80)}`);
    expect(offenders).toEqual([]);
  });

  it('says its state as a tone, never a colour', () => {
    expect(uses.filter(({ tag }) => /\bcolor=/.test(tag)).map(({ file }) => file)).toEqual([]);
  });

  it('has a name, unless it is decorative and has none to give', () => {
    const offenders = uses
      .filter(({ tag }) => (/\bdecorative\b/.test(tag) ? /\b(label|labelledby|valueText)=/.test(tag) : !/\b(label|labelledby)=/.test(tag)))
      .map(({ file, tag }) => `${file}: ${tag.slice(0, 60)}`);
    expect(offenders).toEqual([]);
  });
});

describe('an empty place', () => {
  // Big Picture keeps its own set of components.
  it('is the EmptyState component, not a local paragraph', () => {
    const offenders = Object.entries(frontendSources)
      .filter(([file]) => file.endsWith('.svelte') && file !== path('EmptyState') && !file.startsWith('lib/bigpicture/'))
      .filter(([, source]) => /class="empty[" ]/.test(source))
      .map(([file]) => file);
    expect(offenders).toEqual([]);
  });

  // jsdom applies no component CSS: the rule is read from the stylesheet.
  it('inline, sets no spacing around itself: the place it sits in does', () => {
    const inline = rule(styleOf(frontendSources[path('EmptyState')] ?? ''), '.empty.inline');
    expect(inline).not.toBe('');
    expect(value(inline, 'padding')).toBe('0');
    expect(value(inline, 'margin')).toBeUndefined();
  });

  // The level depends on where the empty state sits, which only the caller knows; inline it has no heading.
  it('on a page or in a dialog, gives its heading level; inline, gives none', () => {
    const offenders = Object.entries(frontendSources)
      .filter(([file]) => file.endsWith('.svelte'))
      .flatMap(([file, source]) => tags(source, 'EmptyState').map((tag) => ({ file, tag })))
      .filter(({ tag }) => /\bsize="inline"/.test(tag) === /\blevel=/.test(tag))
      .map(({ file, tag }) => `${file}: ${tag.slice(0, 70)}`);
    expect(offenders).toEqual([]);
  });
});

describe('every card', () => {
  it('takes its padding as a named size, not a CSS value', () => {
    const offenders = Object.entries(frontendSources)
      .filter(([file]) => file.endsWith('.svelte'))
      .flatMap(([file, source]) =>
        tags(source, 'Card').flatMap((tag) => [...tag.matchAll(/\bpadding=("[^"]*"|\{[^}]*\})/g)].map((match) => `${file}: ${match[1]}`)),
      )
      .filter((use) => !/: "(md|md-even|sm|sm-even)"$/.test(use));
    expect(offenders).toEqual([]);
  });
});

// The theme editor's preview draws its own copies; each must keep the look of the component it stands for.
describe('the theme preview follows the components', () => {
  const preview = styleOf(frontendSources['routes/settings/ThemePreview.svelte'] ?? '');

  it('progress bar: track, shape and the default tone', () => {
    const bar = styleOf(frontendSources[path('ProgressBar')] ?? '');
    const track = rule(bar, '.track');
    expect(value(track, 'background')).toBeDefined();
    expect(value(rule(preview, '.progress'), 'background')).toBe(value(track, 'background'));
    expect(value(rule(preview, '.progress'), 'border-radius')).toBe(value(track, 'border-radius'));
    expect(value(rule(preview, '.progress-fill'), 'border-radius')).toBe(value(rule(bar, '.fill'), 'border-radius'));
    expect(value(rule(preview, '.progress-fill'), 'background')).toBe(value(track, '--bar'));
  });

  it('status badge: shape, type and the tone colours', () => {
    const badge = styleOf(frontendSources[path('StatusBadge')] ?? '');
    for (const property of ['border-radius', 'font-size', 'font-weight']) {
      expect(value(rule(badge, '.badge'), property), property).toBeDefined();
      expect(value(rule(preview, '.badge'), property), property).toBe(value(rule(badge, '.badge'), property));
    }
    for (const tone of ['success', 'warning', 'danger']) {
      expect(value(rule(preview, `.badge.${tone}`), 'background'), tone).toBe(value(rule(badge, `.${tone}`), 'background'));
      expect(value(rule(preview, `.badge.${tone}`), 'color'), tone).toBe(value(rule(badge, `.${tone}`), 'color'));
    }
  });
});

