import { describe, expect, it } from 'vitest';
import { frontendSources } from '../../lib/testing/sources';

/**
 * The showcase shows every base component, so a component added later appears there too.
 *
 * Which components are base is a decision, not something a file can be read for, so the list is kept
 * here by hand. What can be read is the shape a base component has: it holds no app state, its
 * imports reaching outside lib/components only into infrastructure (i18n, layers, utils, motion,
 * theme, confirm). Every component of that shape is either listed as base or named below with why it
 * is a part of a feature instead; a new one fails until it is put on one side, and a base one until it
 * is on the page. A base component that does hold state (HexColorField raises a toast, Toasts reads
 * the store) is listed by hand like the rest.
 */
const BASE = [
  'Button',
  'IconButton',
  'TextLink',
  'Chip',
  'StatusBadge',
  'CountBadge',
  'Checkbox',
  'Toggle',
  'Select',
  'SegmentedControl',
  'Tabs',
  'SearchInput',
  'HexColorField',
  'IntegerInput',
  'Notice',
  'EmptyState',
  'Spinner',
  'ProgressBar',
  'StatTile',
  'Card',
  'Page',
  'PageHeader',
  'Tooltip',
  'Modal',
  'ConfirmModal',
  'DropdownMenu',
  'ContextMenu',
  'Popover',
  'Toasts',
];

/** Shown by what the page does rather than by a tag: the shell mounts the toast stack, the page raises toasts. */
const SHOWN_BY: Record<string, RegExp> = { Toasts: /\btoast\(/ };

const NOT_BASE: Record<string, string> = {
  Artwork: "a game's picture with its fallbacks, drawn from the game's data",
  AvatarCropModal: "the profile's avatar cropper",
  BannerCropModal: "the profile's banner cropper",
  CloudConsentModal: 'a dialog of the cloud saves flow, built on Modal',
  CloudDisableModal: 'a dialog of the cloud saves flow, built on Modal',
  ColorPalette: "the accent picker's swatches in settings and the profile",
  GameHero: "the game page's hero",
  MaskedEmail: "the profile's e-mail and its reveal",
  MenuList: 'the body of DropdownMenu and ContextMenu, shown through them',
  RateLimitInput: 'the download speed limit field of the settings',
  SavesCandidatesModal: 'the save folder picker of the game menu',
  TagEditor: 'the tag list of the collection and tag dialogs, built on Chip',
  ThemeContrast: "the theme editor's contrast report",
};

const COMPONENTS = 'lib/components/';
const SHOWCASE = 'routes/showcase/';

const components = Object.keys(frontendSources)
  .filter((path) => path.startsWith(COMPONENTS) && path.endsWith('.svelte') && !path.slice(COMPONENTS.length).includes('/'))
  .map((path) => path.slice(COMPONENTS.length, -'.svelte'.length));

const INFRASTRUCTURE = /^\.\.\/(i18n|layers|utils|motion|theme|confirm)(\/|$)/;

/** Its imports leave lib/components only for infrastructure: no store, service, binding or feature module. */
function stateless(source: string): boolean {
  const imports = [...source.matchAll(/\bfrom\s+'([^']+)'/g)].map((m) => m[1]);
  return imports.every((from) => !from.startsWith('.') || from.startsWith('./') || INFRASTRUCTURE.test(from));
}

const markup = (source: string) => source.replace(/<(script|style)\b[\s\S]*?<\/\1>/g, '');

/** The base components `pages` (path → source) does not show. */
function missingFrom(pages: Record<string, string>): string[] {
  const texts = Object.values(pages);
  return BASE.filter((name) => {
    const by = SHOWN_BY[name];
    if (by) return !texts.some((text) => by.test(text));
    return !texts.some((text) => new RegExp(`<${name}\\b`).test(markup(text)));
  });
}

const showcase = Object.fromEntries(Object.entries(frontendSources).filter(([path]) => path.startsWith(SHOWCASE) && path.endsWith('.svelte')));

describe('the showcase covers the base components', () => {
  it('reads the showcase and the components', () => {
    expect(Object.keys(showcase)).toContain(`${SHOWCASE}Showcase.svelte`);
    expect(components).toContain('Button');
  });

  it('shows every base component', () => {
    expect(missingFrom(showcase)).toEqual([]);
  });

  it('sees a component taken off the page', () => {
    const without = Object.fromEntries(Object.entries(showcase).map(([path, text]) => [path, text.replaceAll('<StatTile', '<div')]));
    expect(missingFrom(without)).toEqual(['StatTile']);
    expect(missingFrom({ 'x.svelte': '<script>toast("x");</script>' })).not.toContain('Toasts');
    expect(missingFrom({ 'x.svelte': '<script>import Button from "x";</script>' })).toContain('Button');
  });

  it('names only components that exist, each on one side', () => {
    const known = new Set(components);
    expect([...BASE, ...Object.keys(NOT_BASE)].filter((name) => !known.has(name))).toEqual([]);
    expect(BASE.filter((name) => name in NOT_BASE)).toEqual([]);
  });

  it('puts every component shaped like a base one on a side: base and shown, or a named part of a feature', () => {
    const listed = new Set([...BASE, ...Object.keys(NOT_BASE)]);
    const unplaced = components.filter((name) => stateless(frontendSources[`${COMPONENTS}${name}.svelte`]) && !listed.has(name));
    expect(unplaced).toEqual([]);
  });

  it('tells a stateless component from one that holds app state', () => {
    expect(stateless("import X from './X.svelte';\nimport { msg } from '../i18n';\nimport { layer } from '../layers';")).toBe(true);
    expect(stateless("import { toast } from '../stores/toasts';")).toBe(false);
    expect(stateless("import { list } from '../services/library';")).toBe(false);
  });
});
