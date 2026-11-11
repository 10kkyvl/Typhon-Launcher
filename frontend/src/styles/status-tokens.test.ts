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

function tags(source: string, name: string): string[] {
  return [...source.matchAll(new RegExp(`<${name}\\b[^>]*?/>`, 'g'))].map((match) => match[0]);
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

  it('is found where the app draws one', () => {
    expect(uses.length).toBeGreaterThan(20);
  });

  it('reads a tag the way the rules below need', () => {
    expect(tags('<ProgressBar value={1} color="red" />', 'ProgressBar')).toHaveLength(1);
    expect(/\bcolor=/.test('<ProgressBar value={1} color="red" />')).toBe(true);
    expect(/\b(label|labelledby)=/.test('<ProgressBar value={1} />')).toBe(false);
  });

  it('says its state as a tone, never a colour', () => {
    expect(uses.filter(({ tag }) => /\bcolor=/.test(tag)).map(({ file }) => file)).toEqual([]);
  });

  it('has a name', () => {
    expect(uses.filter(({ tag }) => !/\b(label|labelledby)=/.test(tag)).map(({ file, tag }) => `${file}: ${tag.slice(0, 60)}`)).toEqual([]);
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
});

describe('every card', () => {
  it('takes its padding as a named size, not a CSS value', () => {
    const offenders = Object.entries(frontendSources)
      .filter(([file]) => file.endsWith('.svelte'))
      .flatMap(([file, source]) => [...source.matchAll(/<Card\b[^>]*?\bpadding=("[^"]*"|\{[^}]*\})/g)].map((match) => `${file}: ${match[1]}`))
      .filter((use) => !/: "(md|md-even|sm|sm-even)"$/.test(use));
    expect(offenders).toEqual([]);
  });
});

// The theme editor's preview draws its own copies; each must keep the look of the component it stands for.
describe('the theme preview follows the components', () => {
  const preview = styleOf(frontendSources['routes/settings/ThemePreview.svelte'] ?? '');
  const rule = (css: string, selector: string) =>
    new RegExp(`(?:^|\\})\\s*${selector.replace(/[.[\]'=]/g, '\\$&')}\\s*\\{([^}]*)\\}`).exec(css.replace(/\/\*[\s\S]*?\*\//g, ''))?.[1] ?? '';
  const value = (body: string, property: string) => new RegExp(`(?:^|;|\\s)${property}:\\s*([^;]+);`).exec(body)?.[1].trim();

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

