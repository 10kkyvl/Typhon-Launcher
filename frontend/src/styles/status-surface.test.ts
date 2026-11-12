import { describe, expect, it } from 'vitest';
import { frontendSources, goSources } from '../lib/testing/sources';
import { luminance, rgb } from '../lib/theme/accent';

/**
 * Status messages sit on a surface. The window backdrop is the game's art, and a status colour straight
 * on it does not keep its contrast, so an error, a warning or a success is shown on the opaque plate of
 * a Notice (or on a surface the page already has), never as coloured text on the bare page.
 *
 * The guard below reads source text: it is a tripwire, not proof. It sees the rules of every component
 * and stylesheet, a status colour anywhere in a `color` value (mixed, with a fallback, or through a
 * local custom property), the last of repeated declarations, and inline `style="color: …"` and
 * `style:color`. It does not see, and these stay for review and the live check:
 * - a rule nested inside another rule's block;
 * - a custom property set inline on an element (`style:--x=…`) and then read as `color`;
 * - a colour handed to a child component as a prop;
 * - a plate cancelled by a later rule with a stronger selector or from another file;
 * - text painted with `fill` or `-webkit-text-fill-color` instead of `color`.
 */

type RGB = [number, number, number];
const BLACK: RGB = [0, 0, 0];

function presets(): Map<string, Record<string, string>> {
  const source = goSources['internal/theme/presets.go'];
  expect(source).toBeTypeOf('string');
  const out = new Map<string, Record<string, string>>();
  for (const m of source.matchAll(/^var preset\w+ = Theme\{([\s\S]*?)^\}/gm)) {
    const id = /\bID:\s*"([^"]+)"/.exec(m[1])?.[1];
    expect(id).toBeTypeOf('string');
    out.set(id as string, Object.fromEntries([...m[1].matchAll(/"(--[\w-]+)":\s*"([^"]*)"/g)].map((t) => [t[1], t[2]])));
  }
  return out;
}

function ratio(a: RGB, b: RGB): number {
  const [x, y] = [luminance(a), luminance(b)];
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

const mix = (a: RGB, b: RGB, share: number): RGB => [0, 1, 2].map((i) => a[i] * share + b[i] * (1 - share)) as RGB;

const NOTICE = 'lib/components/Notice.svelte';

function styleOf(path: string): string {
  const source = frontendSources[path];
  expect(source, path).toBeTypeOf('string');
  return /<style[^>]*>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '';
}

describe('the plate of a notice', () => {
  const css = styleOf(NOTICE);
  const plate = /\.notice\s*\{([^}]*)\}/.exec(css)?.[1] ?? '';
  const fill = /background:\s*color-mix\(in srgb, var\(--notice-tone\) (\d+)%, var\((--[\w-]+)\)\)/.exec(plate);
  const tones = Object.fromEntries(
    [...css.matchAll(/\.notice\[data-tone='(\w+)'\]\s*\{\s*--notice-tone:\s*var\((--[\w-]+)\)/g)].map((m) => [m[1], m[2]]),
  );
  const base = /--notice-tone:\s*var\((--[\w-]+)\)/.exec(plate)?.[1];
  // Every colour the notice paints its words in, read from its rules: the icon is in the tone (3:1).
  const words = [
    ...new Set(
      [...css.matchAll(/([^{}]+)\{([^}]*)\}/g)].flatMap((m) =>
        [...m[2].matchAll(/(?:^|[^-\w])color:\s*var\((--[\w-]+)\)/g)].map((c) => c[1]).filter((name) => name !== '--notice-tone'),
      ),
    ),
  ];

  it('is the tone mixed into an opaque surface, never a translucent tint', () => {
    expect(fill, plate).not.toBeNull();
    expect((fill as RegExpExecArray)[2]).toMatch(/^--surface(-\d)?$/);
    expect(plate).not.toMatch(/-subtle|rgba\(|transparent|opacity/);
    expect(base).toBe('--accent-bright');
    expect(tones).toEqual({ success: '--success', warning: '--warning', danger: '--danger' });
    expect(words).toContain('--text');
  });

  for (const id of ['dark', 'light', 'contrast', 'blush']) {
    it(`${id} keeps a notice's text at 4.5:1 and its icon at 3:1 on the plate, whatever the tone`, () => {
      const tokens = presets().get(id);
      expect(tokens, id).toBeDefined();
      const t = tokens as Record<string, string>;
      const share = Number((fill as RegExpExecArray)[1]) / 100;
      const surface = rgb(t[(fill as RegExpExecArray)[2]], rgb(t['--bg'], BLACK));
      const failures: string[] = [];
      for (const [tone, token] of Object.entries({ info: base as string, ...tones })) {
        const colour = rgb(t[token], surface);
        const on = mix(colour, surface, share);
        for (const name of words) {
          const r = ratio(rgb(t[name], on), on);
          if (r < 4.5) failures.push(`${tone}: ${name} ${r.toFixed(2)} < 4.5`);
        }
        const icon = ratio(colour, on);
        if (icon < 3) failures.push(`${tone}: icon ${icon.toFixed(2)} < 3`);
      }
      expect(failures).toEqual([]);
    });
  }

  it('paints its text in the text colours and only its icon in the tone', () => {
    const coloured = [...css.matchAll(/([^{}]+)\{([^}]*)\}/g)]
      .filter((m) => /(^|[^-])color:\s*var\(--notice-tone\)/.test(m[2]))
      .map((m) => m[1].trim());
    expect(coloured).toEqual(['.icon']);
    expect(css).not.toMatch(/(^|[^-])color:\s*var\(--(danger|warning|success)\)/);
  });
});

// A status colour anywhere in a value: on its own, with a fallback, or mixed into another colour.
const STATUS = /var\(--(danger|warning|success)(?![\w-])/;
const STATUS_TOKEN = /^--(danger|warning|success)$/;
// An opaque fill: a surface, or a tone mixed into one (the way a notice and a badge fill themselves), in
// any interpolation space color-mix() takes.
const SURFACE = String.raw`var\((--surface(?:-\d)?|--bg)\)`;
const SPACE = String.raw`[a-z][a-z0-9-]*(?: (?:shorter|longer|increasing|decreasing) hue)?`;
const PLATE = new RegExp(String.raw`^(?:${SURFACE}|color-mix\(in (${SPACE}), var\((--[\w-]+)\) (\d+)%, ${SURFACE}\))$`);

interface Plate {
  surface: string;
  tone?: string;
  share?: number;
  space?: string;
}

// Colour mixing the way color-mix() does it, in the spaces the guard can measure.
const toLinear = (v: number) => {
  const s = v / 255;
  return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
};
const toByte = (v: number) => 255 * (v <= 0.0031308 ? v * 12.92 : 1.055 * Math.max(0, v) ** (1 / 2.4) - 0.055);

function toOklab([r, g, b]: RGB): RGB {
  const [lr, lg, lb] = [r, g, b].map(toLinear);
  const l = Math.cbrt(0.4122214708 * lr + 0.5363325363 * lg + 0.0514459929 * lb);
  const m = Math.cbrt(0.2119034982 * lr + 0.6806995451 * lg + 0.1073969566 * lb);
  const s = Math.cbrt(0.0883024619 * lr + 0.2817188376 * lg + 0.6299787005 * lb);
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
  ];
}

function fromOklab([L, a, b]: RGB): RGB {
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ].map(toByte) as RGB;
}

function mixIn(space: string, a: RGB, b: RGB, share: number): RGB | null {
  if (space === 'srgb') return mix(a, b, share);
  if (space === 'srgb-linear') return mix(a.map(toLinear) as RGB, b.map(toLinear) as RGB, share).map(toByte) as RGB;
  if (space === 'oklab') return fromOklab(mix(toOklab(a), toOklab(b), share));
  return null;
}

// The declarations of a rule in order; a property declared twice is what the last one says.
function declarations(body: string): Map<string, string> {
  const out = new Map<string, string>();
  for (const part of body.split(';')) {
    const at = part.indexOf(':');
    if (at <= 0) continue;
    const name = part.slice(0, at).trim().toLowerCase();
    if (name === 'background' || name === 'background-color') out.delete('background');
    out.delete(name);
    out.set(name === 'background-color' ? 'background' : name, part.slice(at + 1).trim());
  }
  return out;
}

function plateOf(body: string): Plate | null {
  const m = PLATE.exec(declarations(body).get('background') ?? '');
  if (!m) return null;
  return m[1] ? { surface: m[1] } : { surface: m[5], space: m[2], tone: m[3], share: Number(m[4]) / 100 };
}

// The colour a plate shows in a preset, with its tone already resolved to a theme token; null when the
// mix is in a space the guard does not measure.
function fill(t: Record<string, string>, plate: Plate, tone = plate.tone): RGB | null {
  const surface = rgb(t[plate.surface], rgb(t['--bg'], BLACK));
  if (!tone) return surface;
  expect(t[tone], `${tone} is a theme colour`).toBeTypeOf('string');
  return mixIn(plate.space ?? 'srgb', rgb(t[tone], surface), surface, plate.share as number);
}

// Whole areas where every status text has an opaque surface under it.
const AREAS: [RegExp, string][] = [
  [/Modal\.svelte$/, 'a dialog'],
  [/^routes\/settings\//, 'the cards of the settings page'],
  [/^routes\/auth\//, 'the sign-in screen, which paints its own page'],
  [/^lib\/bigpicture\//, 'Big Picture, which draws no backdrop: its screens are its own opaque page'],
  [/^overlay\//, 'the in-game overlay, which draws no backdrop: its panels are opaque'],
];

// A status colour without a plate of its own: the surface it sits on, then the rules. Marks and icons
// are not text (3:1); a badge carries a fill of its own (see below). A new entry needs its surface named.
const ON_SURFACE: Record<string, [surface: string, ...rules: string[]]> = {
  'routes/diskusage/DiskUsage.svelte': ['Card', '.reason'],
  'routes/game/ReviewComposer.svelte': ["the plate of one's own review", '.recommend.negative'],
  'routes/profile/Profile.svelte': ['the editing bar', '.topbar-sub.error'],
  'routes/profile/ProfileHeader.svelte': ['the profile canvas', '.error'],
  'routes/history/History.svelte': ['the table', '.row.failed .icon-cell', '.row.failed .detail'],
  'routes/sources/Sources.svelte': [
    'the table',
    '.source-error',
    '.insecure',
    '.count.review',
  ],
  'routes/friends/FriendCodeCard.svelte': ['Card', '.mark'],
  'routes/downloads/Downloads.svelte': ['the fill of a row', '.error-text'],
  'lib/components/AccountMenu.svelte': ['the header island, checked over the wash in theme-contrast.test.ts', '.offline-mark'],
  'lib/components/SidebarStatus.svelte': ['the sidebar island; icons', '.warn :global(svg)', '.danger :global(svg)'],
  'lib/components/UpdateCard.svelte': ['Card', '.mark.cautious', '.explain.cautious'],
  'lib/components/VerifyCard.svelte': ['Card', '.error'],
  'lib/components/FeedNote.svelte': ['Card of the feed', '.left.low'],
  'lib/components/ThemeContrast.svelte': ['Card of the theme editor', '.summary', '.summary.bad', '.icon', 'li.bad .icon'],
  'lib/components/IntegerInput.svelte': ['the cards of the settings page', '.error'],
  'lib/components/RateLimitInput.svelte': ['the cards of the settings page', '.error'],
  'lib/components/ReleaseNotesList.svelte': ['a dialog; badges', '.kind-added', '.kind-fixed', '.kind-removed'],
  'lib/components/ActivityDock.svelte': ['the dock panel; an icon', '.row-icon.attention'],
  'lib/components/TopbarSearch.svelte': ['the results panel', '.results-note.error'],
  'lib/components/MenuList.svelte': ['a menu', ".item.danger:not([aria-disabled='true'])"],
  'lib/components/Toasts.svelte': [
    'a toast; icons',
    '.toast.success > :global(svg)',
    '.toast.warning > :global(svg)',
    '.toast.danger > :global(svg)',
  ],
  'lib/components/UpdateOverlay.svelte': ['the card of the update screen', '.card-danger', '.card-danger .meta'],
  'lib/components/LegalAcceptanceScreen.svelte': ['the card of the screen', '.error'],
  'lib/components/TelemetryConsentScreen.svelte': ['the card of the screen', '.error'],
  'lib/components/AvatarEditor.svelte': ['the profile canvas', '.error'],
  'lib/components/profile/BlockSettings.svelte': ['the settings panel of a block', '.error'],
  'lib/components/profile/CollectionBlock.svelte': ['the scrim over a cover; marks', '.heart', '.mastered'],
  'lib/components/SystemRequirements.svelte': [
    'the plate of the checks',
    '.mark.success, .have.success',
    '.mark.warning, .have.warning',
    '.mark.danger, .have.danger',
  ],
  'lib/components/Button.svelte': [
    'a control, not a message; in the overlay, its opaque panels',
    '.danger',
    ":global([data-controls='1x']) .danger",
    ":global([data-controls='1x']) .danger:hover:not(:disabled, [aria-disabled='true'])",
  ],
  'lib/components/Notice.svelte': ['the plate of the notice; its icon', '.icon'],
  'lib/components/DownloadItem.svelte': ['the fill of a download row', '.stalled'],
};

// Where the surface named above is a rule of the same component, that rule has to stay opaque.
const LOCAL_SURFACE: Record<string, string> = {
  'routes/game/ReviewComposer.svelte': '.mine',
  'routes/profile/Profile.svelte': '.topbar',
  'routes/history/History.svelte': '.table',
  'routes/sources/Sources.svelte': '.table',
  'routes/downloads/Downloads.svelte': '.row',
  'lib/components/ActivityDock.svelte': '.panel',
  'lib/components/TopbarSearch.svelte': '.results',
  'lib/components/profile/BlockSettings.svelte': '.settings',
  'lib/components/SystemRequirements.svelte': '.checks',
  'lib/components/Notice.svelte': '.notice',
  'lib/components/DownloadItem.svelte': '.item',
};

interface StatusRule {
  path: string;
  selector: string;
  /** The status colours the rule's `color` can resolve to. */
  statuses: string[];
  /** The variable its `color` reads, when it reads one: a local one changes with the plate's tone. */
  colourVar?: string;
  plate: Plate | null;
  locals: Map<string, Set<string>>;
}

// Custom properties a stylesheet declares, each with every value it can take, resolved through the
// others: `--late-tone: var(--danger)` read as `var(--late-tone)` is --danger.
function localsOf(css: string): Map<string, Set<string>> {
  const raw = new Map<string, Set<string>>();
  for (const m of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    for (const [name, value] of declarations(m[2])) {
      if (!name.startsWith('--')) continue;
      const token = /^var\((--[\w-]+)\s*(?:,[^)]*)?\)$/.exec(value)?.[1] ?? value;
      (raw.get(name) ?? raw.set(name, new Set()).get(name))?.add(token);
    }
  }
  const resolve = (value: string, seen: Set<string>): string[] =>
    raw.has(value) && !seen.has(value) ? [...(raw.get(value) as Set<string>)].flatMap((v) => resolve(v, new Set([...seen, value]))) : [value];
  return new Map([...raw.keys()].map((name) => [name, new Set(resolve(name, new Set()))]));
}

const resolved = (name: string, locals: Map<string, Set<string>>) => [...(locals.get(name) ?? [name])];

// The status colours a colour value can be: named in it, or carried by a local property it reads.
function statusesIn(value: string, locals: Map<string, Set<string>>): string[] {
  const out = new Set<string>();
  const direct = STATUS.exec(value)?.[1];
  if (direct) out.add(`--${direct}`);
  for (const m of value.matchAll(/var\((--[\w-]+)/g)) {
    for (const token of resolved(m[1], locals)) {
      if (STATUS_TOKEN.test(token)) out.add(token);
      const inner = STATUS.exec(token)?.[1];
      if (inner) out.add(`--${inner}`);
    }
  }
  return [...out];
}

const tidy = (text: string) => text.trim().replace(/\s+/g, ' ');

// Plain stylesheets, scanned like the style blocks of components. The test run empties CSS imports, so
// the glob only finds them and the text is read from disk.
type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const STYLESHEETS = Object.fromEntries(
  Object.keys(import.meta.glob('../**/*.css')).map((path) => [
    path.replace(/^\.\.\//, '').replace(/^\.\//, 'styles/'),
    fs.readFileSync(new URL(path, import.meta.url), 'utf8'),
  ]),
);

function cssRules(path: string, css: string, out: StatusRule[]) {
  const locals = localsOf(css);
  for (const m of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const colour = declarations(m[2]).get('color') ?? '';
    const statuses = statusesIn(colour, locals);
    if (statuses.length === 0) continue;
    const colourVar = /^var\((--[\w-]+)/.exec(colour)?.[1];
    out.push({ path, selector: tidy(m[1]), statuses, colourVar, plate: plateOf(m[2]), locals });
  }
  return locals;
}

// A source tripwire, not a CSS engine: every rule of every component and stylesheet whose `color` is a
// status colour, with the plate the same rule gives it, and status colours set inline in the markup.
function statusRules(): StatusRule[] {
  const out: StatusRule[] = [];
  for (const [path, css] of Object.entries(STYLESHEETS)) cssRules(path, css.replace(/\/\*[\s\S]*?\*\//g, ''), out);
  for (const [path, source] of Object.entries(frontendSources)) {
    if (!path.endsWith('.svelte')) continue;
    const css = (/<style[^>]*>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '').replace(/\/\*[\s\S]*?\*\//g, '');
    const locals = cssRules(path, css, out);
    const script = [...source.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g)].map((m) => m[1]).join('\n');
    const markup = source.replace(/<script[^>]*>[\s\S]*?<\/script>|<style[^>]*>[\s\S]*?<\/style>/g, '');
    for (const m of markup.matchAll(/\bstyle="([^"]*)"/g)) {
      const statuses = statusesIn(declarations(m[1]).get('color') ?? '', locals);
      if (statuses.length) out.push({ path, selector: tidy(m[0]), statuses, plate: null, locals });
    }
    // An expression is opaque to the tripwire: it counts when the component's script spells a status colour.
    const spelled = STATUS.exec(script)?.[1];
    for (const m of markup.matchAll(/\bstyle:color(?:\|important)?=(?:"([^"]*)"|\{([^}]*)\})/g)) {
      const statuses = statusesIn(m[1] ?? m[2], locals);
      if (!statuses.length && m[2] !== undefined && spelled) statuses.push(`--${spelled}`);
      if (statuses.length) out.push({ path, selector: tidy(m[0]), statuses, plate: null, locals });
    }
  }
  return out;
}

// The pairs to measure on a rule's own mixed plate: its colour and the tone of its fill, which move
// together when both read the same local property.
function measured(rule: StatusRule): [colour: string, tone: string][] {
  const tone = rule.plate?.tone as string;
  if (rule.colourVar && rule.colourVar === tone && rule.locals.has(tone)) {
    return resolved(tone, rule.locals).filter((v) => STATUS_TOKEN.test(v)).map((v) => [v, v]);
  }
  return rule.statuses.flatMap((colour) => resolved(tone, rule.locals).map((t): [string, string] => [colour, t]));
}

const FIX =
  'status-coloured text with no opaque surface under it: give the rule a plate of its own (background: ' +
  'var(--surface-N) or color-mix(in srgb, var(--tone) N%, var(--surface-N))), or name the surface it sits on ' +
  'in ON_SURFACE in styles/status-surface.test.ts (and the rule in LOCAL_SURFACE when that surface is a rule ' +
  'of the same component)';

describe('status-coloured text in the pages', () => {
  const rules = statusRules();
  // The areas above draw no backdrop or are surfaces as a whole; the rest is what sits over the art.
  const paged = rules.filter((rule) => !AREAS.some(([area]) => area.test(rule.path)));
  const loose = paged.filter((rule) => !rule.plate);

  it('sits on a plate of its own or on a named surface, never on the bare page', () => {
    expect(rules.length).toBeGreaterThan(40);
    const bare = loose
      .filter((rule) => !ON_SURFACE[rule.path]?.slice(1).includes(rule.selector))
      .map((rule) => `${rule.path}: ${rule.selector}`);
    expect(bare, FIX).toEqual([]);
  });

  it('names no surface for a rule that is gone', () => {
    const live = new Set(loose.map((rule) => `${rule.path}: ${rule.selector}`));
    const stale = Object.entries(ON_SURFACE)
      .flatMap(([path, [, ...selectors]]) => selectors.map((selector) => `${path}: ${selector}`))
      .filter((entry) => !live.has(entry));
    expect(stale, 'remove these from ON_SURFACE: the rule is gone or has a plate of its own now').toEqual([]);
  });

  it('keeps the surfaces it names inside a component opaque', () => {
    const open: string[] = [];
    for (const [path, selector] of Object.entries(LOCAL_SURFACE)) {
      expect(ON_SURFACE[path], path).toBeDefined();
      const css = styleOf(path).replace(/\/\*[\s\S]*?\*\//g, '');
      const rule = new RegExp(`(?:^|})\\s*${selector.replace('.', '\\.')}\\s*\\{([^}]*)\\}`).exec(css)?.[1];
      if (!plateOf(rule ?? '')) open.push(`${path}: ${selector}`);
    }
    expect(open, 'the surface named in LOCAL_SURFACE lost its opaque fill: give it one back, or name the real surface').toEqual([]);
  });

  for (const id of ['dark', 'light', 'contrast', 'blush']) {
    it(`${id} keeps a status colour at 4.5:1 on every plate the pages give it`, () => {
      const t = presets().get(id) as Record<string, string>;
      expect(t, id).toBeDefined();
      const surfaces = [...new Set(paged.flatMap((rule) => (rule.plate && !rule.plate.tone ? [rule.plate.surface] : [])))];
      expect(surfaces.length).toBeGreaterThan(0);
      const failures: string[] = [];
      for (const surface of surfaces) {
        const on = fill(t, { surface }) as RGB;
        for (const name of ['--danger', '--warning', '--success']) {
          const r = ratio(rgb(t[name], on), on);
          if (r < 4.5) failures.push(`${name} on ${surface}: ${r.toFixed(2)}`);
        }
      }
      // A fill mixed from a tone is measured with the colour its own rule sets on it.
      for (const rule of paged) {
        if (!rule.plate?.tone) continue;
        for (const [colour, tone] of measured(rule)) {
          const on = fill(t, rule.plate, tone);
          if (!on) {
            failures.push(`${rule.path}: ${rule.selector}: cannot measure a mix in ${rule.plate.space}; mix in srgb, srgb-linear or oklab`);
            continue;
          }
          const r = ratio(rgb(t[colour], on), on);
          if (r < 4.5) failures.push(`${rule.path}: ${rule.selector}: ${colour} on ${tone} ${r.toFixed(2)}`);
        }
      }
      expect(failures, 'raise the contrast: a lower share of the tone in the fill, or a stronger status colour').toEqual([]);
    });
  }
});

// A badge sits wherever its row does, the bare page included (a ticket's status in the head of a
// ticket page): its label is text on its own fill, and that fill is opaque, so the label keeps its
// contrast over any backdrop.
const BADGES: Record<string, string[]> = {
  'lib/components/StatusBadge.svelte': ['.success', '.warning', '.danger', '.accent', '.neutral'],
};

// The border colour a badge kind ends up with: its own rule's, else the base `.badge` rule's.
function borderOf(css: string, selector: string): Plate | null {
  const edge = (sel: string) => {
    const body = new RegExp(`(?:^|})\\s*${sel.replace(/\./g, '\\.')}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? '';
    const rule = declarations(body);
    return rule.get('border-color') ?? rule.get('border');
  };
  const value = edge(selector) ?? edge('.badge') ?? '';
  const m = /color-mix\(in srgb, var\((--[\w-]+)\) (\d+)%, var\((--surface(?:-\d)?)\)\)/.exec(value);
  return m ? { tone: m[1], share: Number(m[2]) / 100, surface: m[3] } : null;
}

describe('the fill of a badge', () => {
  const kinds = Object.entries(BADGES).flatMap(([path, selectors]) => {
    const css = styleOf(path).replace(/\/\*[\s\S]*?\*\//g, '');
    return selectors.map((selector) => {
      const body = new RegExp(`(?:^|})\\s*${selector.replace(/\./g, '\\.')}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? '';
      return {
        kind: `${path}: ${selector}`,
        plate: plateOf(body),
        label: /(?:^|[^-\w])color:\s*var\((--[\w-]+)\)/.exec(body)?.[1],
        edge: borderOf(css, selector),
      };
    });
  });

  it('is opaque: a surface, or the tone mixed into one, never a translucent tint', () => {
    expect(kinds.filter((k) => !k.plate || !k.label).map((k) => k.kind)).toEqual([]);
  });

  // In the light preset the fill a label allows is a shade off the page; a 1px border in the badge's
  // tone, mixed into --surface-2 the way a notice draws its own, carries the shape.
  it('is outlined in its own tone, the way a notice is', () => {
    const notice = /border:\s*1px solid color-mix\(in srgb, var\(--notice-tone\) (\d+)%, var\(--surface-2\)\)/.exec(styleOf(NOTICE));
    expect(notice).not.toBeNull();
    const wrong = kinds
      .filter((k) => k.edge?.tone !== (k.plate?.tone ?? '--text') || k.edge?.share !== Number((notice as RegExpExecArray)[1]) / 100)
      .map((k) => `${k.kind}: ${k.edge ? `${k.edge.tone} ${k.edge.share}` : 'no border'}`);
    expect(wrong).toEqual([]);
  });

  // The border sits inside the badge: its 1px comes off the padding of the rule that draws it.
  it.each(Object.keys(BADGES))('%s keeps its outer size with the border', (path) => {
    const css = styleOf(path).replace(/\/\*[\s\S]*?\*\//g, '');
    const base = declarations(/(?:^|})\s*\.badge\s*\{([^}]*)\}/.exec(css)?.[1] ?? '');
    expect(base.get('border') ?? '').toMatch(/^1px solid\b/);
    const padding = base.get('padding') ?? '';
    const sides = padding.match(/calc\([^()]*(?:\([^()]*\))?[^()]*\)|[^\s]+/g) ?? [];
    expect(sides.length, padding).toBeGreaterThan(0);
    expect(sides.filter((side) => side !== '0' && !/^calc\(.* - 1px\)$/.test(side)), padding).toEqual([]);
  });

  for (const id of ['dark', 'light', 'contrast', 'blush']) {
    it(`${id} keeps a badge's label at 4.5:1 on its fill`, () => {
      const t = presets().get(id) as Record<string, string>;
      expect(t, id).toBeDefined();
      const failures: string[] = [];
      for (const { kind, plate, label } of kinds) {
        if (!plate || !label) continue;
        const on = fill(t, plate);
        if (!on) {
          failures.push(`${kind}: a fill the guard cannot measure`);
          continue;
        }
        const r = ratio(rgb(t[label], on), on);
        if (r < 4.5) failures.push(`${kind}: ${label} ${r.toFixed(2)}`);
      }
      expect(failures).toEqual([]);
    });
  }
});

// A tripwire on the styles, not proof of layout: a field counter that turns red at its limit stays next to
// its field on a plate, and its state changes only colour and fill, so it does not shift when the limit
// is crossed (the box comes from the counter's own rule).
describe('tripwire: a field counter at its limit', () => {
  const states = Object.entries(frontendSources).flatMap(([path, source]) => {
    if (!path.endsWith('.svelte')) return [];
    const css = (/<style[^>]*>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '').replace(/\/\*[\s\S]*?\*\//g, '');
    return [...css.matchAll(/([^{}]+)\{([^{}]*)\}/g)]
      .filter((m) => /\.counter\.[\w-]+/.test(m[1]) && STATUS.test(declarations(m[2]).get('color') ?? ''))
      .map((m) => ({ rule: `${path}: ${tidy(m[1])}`, body: m[2] }));
  });

  it('sits on a plate', () => {
    expect(states.length).toBeGreaterThanOrEqual(5);
    expect(states.filter((s) => !plateOf(s.body)).map((s) => s.rule)).toEqual([]);
  });

  it('keeps the box of the counter in both states', () => {
    const box = /^(?:padding|margin|border(?!-radius)|width|height|min-|max-|font-size|line-height|display)/;
    const moved = states.flatMap((s) => [...declarations(s.body).keys()].filter((name) => box.test(name)).map((name) => `${s.rule}: ${name}`));
    expect(moved).toEqual([]);
  });
});

// The review composer's two pressed toggles are one pair: the same opaque fill, each in its own tone.
describe('the pressed toggles of the review composer', () => {
  const css = styleOf('routes/game/ReviewComposer.svelte').replace(/\/\*[\s\S]*?\*\//g, '');
  const body = (selector: string) => new RegExp(`(?:^|})\\s*${selector.replace(/\./g, '\\.')}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? '';
  const [yes, no] = ['.toggle.positive.active', '.toggle.negative.active'].map((s) => ({ plate: plateOf(body(s)), rule: declarations(body(s)) }));

  it('fill alike: each its tone mixed into the same surface at the same share', () => {
    expect(yes.plate?.tone, 'recommend').toBeTypeOf('string');
    expect(no.plate?.tone, 'not recommended').toBeTypeOf('string');
    expect({ ...yes.plate, tone: undefined }).toEqual({ ...no.plate, tone: undefined });
    expect(yes.rule.get('border-color')).toBe(`var(${yes.plate?.tone})`);
    expect(no.rule.get('border-color')).toBe(`var(${no.plate?.tone})`);
  });

  for (const id of ['dark', 'light', 'contrast', 'blush']) {
    it(`${id} keeps the label of each pressed toggle at 4.5:1 on its fill`, () => {
      const t = presets().get(id) as Record<string, string>;
      expect(t, id).toBeDefined();
      const failures: string[] = [];
      for (const [name, { plate, rule }] of [['recommend', yes], ['not recommended', no]] as const) {
        const label = /var\((--[\w-]+)\)/.exec(rule.get('color') ?? '')?.[1];
        if (!plate || !label) {
          failures.push(`${name}: no fill or label`);
          continue;
        }
        const on = fill(t, plate);
        if (!on) {
          failures.push(`${name}: a fill the guard cannot measure`);
          continue;
        }
        const r = ratio(rgb(t[label], on), on);
        if (r < 4.5) failures.push(`${name}: ${label} ${r.toFixed(2)}`);
      }
      expect(failures).toEqual([]);
    });
  }
});
