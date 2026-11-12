import { describe, expect, it } from 'vitest';
import { frontendSources, goSources } from '../lib/testing/sources';
import { luminance, rgb } from '../lib/theme/accent';

/**
 * Status messages sit on a surface. The window backdrop is the game's art, and a status colour straight
 * on it does not keep its contrast, so an error, a warning or a success is shown on the opaque plate of
 * a Notice (or on a surface the page already has), never as coloured text on the bare page.
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
// An opaque fill: a surface, or a tone mixed into one (the way a notice and a badge fill themselves).
const SURFACE = String.raw`var\((--surface(?:-\d)?|--bg)\)`;
const PLATE = new RegExp(String.raw`^(?:${SURFACE}|color-mix\(in srgb, var\((--[\w-]+)\) (\d+)%, ${SURFACE}\))$`);

interface Plate {
  surface: string;
  tone?: string;
  share?: number;
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
  return m[1] ? { surface: m[1] } : { surface: m[4], tone: m[2], share: Number(m[3]) / 100 };
}

// The colour a plate shows in a preset.
function fill(t: Record<string, string>, plate: Plate): RGB {
  const surface = rgb(t[plate.surface], rgb(t['--bg'], BLACK));
  return plate.tone ? mix(rgb(t[plate.tone], surface), surface, plate.share as number) : surface;
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
  'routes/installed/Installed.svelte': [
    'Card and the fill of a row; an icon on the dark scrim of a badge over a cover',
    '.storage-error',
    '.row-update',
    '.card-badge :global(svg)',
  ],
  'routes/game/ReviewComposer.svelte': ["the plate of one's own review", '.recommend.negative', '.hidden-note'],
  'routes/game/ReviewSummary.svelte': ['Card', '.error'],
  'routes/profile/Profile.svelte': ['the editing bar', '.topbar-sub.error'],
  'routes/profile/ProfileHeader.svelte': ['the profile canvas', '.error'],
  'routes/history/History.svelte': ['the table', '.row.failed .icon-cell', '.row.failed .detail'],
  'routes/sources/Sources.svelte': [
    'the table; the health icon of a row',
    '.source-error',
    '.insecure',
    '.warn-icon',
    '.count.review',
    'style:color={health.color}',
  ],
  'routes/friends/FriendCodeCard.svelte': ['Card', '.mark'],
  'routes/downloads/Downloads.svelte': ['the fill of a row', '.error-text'],
  'lib/components/AccountMenu.svelte': ['the header island, checked over the wash in theme-contrast.test.ts', '.offline-mark'],
  'lib/components/SidebarStatus.svelte': ['the sidebar island; icons', '.warn :global(svg)', '.danger :global(svg)'],
  'lib/components/AchievementsCard.svelte': ['Card', '.note.warn', '.state.error'],
  'lib/components/UpdateCard.svelte': ['Card', '.mark.cautious', '.explain.cautious', '.error'],
  'lib/components/VerifyCard.svelte': ['Card', '.error'],
  'lib/components/FeedNote.svelte': ['Card of the feed', '.left.low'],
  'lib/components/ThemeContrast.svelte': ['Card of the theme editor', '.summary', '.summary.bad', '.icon', 'li.bad .icon'],
  'lib/components/IntegerInput.svelte': ['the cards of the settings page', '.error'],
  'lib/components/RateLimitInput.svelte': ['the cards of the settings page', '.error'],
  'lib/components/GameProfiles.svelte': ['a dialog', '.failure'],
  'lib/components/ReleaseNotesList.svelte': ['a dialog; badges', '.kind-added', '.kind-fixed', '.kind-removed'],
  'lib/components/ChatPanel.svelte': ['the chat panel', '.history-error', '.retry', '.chat-error, .disabled-copy'],
  'lib/components/ActivityDock.svelte': ['the dock panel; an icon', '.row-icon.attention'],
  'lib/components/TopbarSearch.svelte': ['the results panel', '.results-error'],
  'lib/components/MenuList.svelte': ['a menu', ".item.danger:not([aria-disabled='true'])"],
  'lib/components/Toasts.svelte': ['a toast; icons', '.toast.success :global(svg)', '.toast.danger :global(svg)'],
  'lib/components/UpdateOverlay.svelte': ['the card of the update screen', '.card-danger', '.card-danger .meta'],
  'lib/components/LegalAcceptanceScreen.svelte': ['the card of the screen', '.error'],
  'lib/components/TelemetryConsentScreen.svelte': ['the card of the screen', '.error'],
  'lib/components/AvatarEditor.svelte': ['the profile canvas', '.error'],
  'lib/components/profile/BlockGrid.svelte': ['the profile canvas', '.issue-tag'],
  'lib/components/profile/BlockPicker.svelte': ['the profile canvas', '.hint'],
  'lib/components/profile/BlockSettings.svelte': ['the settings panel of a block', '.error'],
  'lib/components/profile/CollectionBlock.svelte': ['the scrim over a cover; marks', '.heart', '.mastered'],
  'lib/components/SystemRequirements.svelte': [
    'the plate of the checks',
    '.mark.success, .have.success',
    '.mark.warning, .have.warning',
    '.mark.danger, .have.danger',
  ],
  'lib/components/Button.svelte': ['a control, not a message', '.danger'],
  'lib/components/Notice.svelte': ['the plate of the notice; its icon', '.icon'],
  'lib/components/DownloadItem.svelte': ['the fill of a download row', '.stalled'],
};

// Where the surface named above is a rule of the same component, that rule has to stay opaque.
const LOCAL_SURFACE: Record<string, string> = {
  'routes/installed/Installed.svelte': '.row',
  'routes/game/ReviewComposer.svelte': '.mine',
  'routes/profile/Profile.svelte': '.topbar',
  'routes/history/History.svelte': '.table',
  'routes/sources/Sources.svelte': '.table',
  'routes/downloads/Downloads.svelte': '.row',
  'lib/components/ChatPanel.svelte': '.chat-panel',
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
  status: string;
  plate: Plate | null;
}

// Custom properties of a component that carry a status colour, directly or through another one.
function carriers(css: string): Map<string, string> {
  const out = new Map<string, string>();
  const custom = [...css.matchAll(/([^{}]+)\{([^{}]*)\}/g)].flatMap((m) =>
    [...declarations(m[2])].filter(([name]) => name.startsWith('--')),
  );
  for (let grew = true; grew; ) {
    grew = false;
    for (const [name, value] of custom) {
      const status = statusIn(value, out);
      if (status && !out.has(name)) {
        out.set(name, status);
        grew = true;
      }
    }
  }
  return out;
}

function statusIn(value: string, carried: Map<string, string>): string | undefined {
  const direct = STATUS.exec(value)?.[1];
  if (direct) return `--${direct}`;
  for (const m of value.matchAll(/var\((--[\w-]+)/g)) if (carried.has(m[1])) return carried.get(m[1]);
  return undefined;
}

const tidy = (text: string) => text.trim().replace(/\s+/g, ' ');

// A source tripwire, not a CSS engine: every rule of every component whose `color` is a status colour,
// with the plate the same rule gives it, and status colours set inline in the markup.
function statusRules(): StatusRule[] {
  const out: StatusRule[] = [];
  for (const [path, source] of Object.entries(frontendSources)) {
    if (!path.endsWith('.svelte')) continue;
    const css = (/<style[^>]*>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '').replace(/\/\*[\s\S]*?\*\//g, '');
    const carried = carriers(css);
    for (const m of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
      const status = statusIn(declarations(m[2]).get('color') ?? '', carried);
      if (status) out.push({ path, selector: tidy(m[1]), status, plate: plateOf(m[2]) });
    }
    const script = [...source.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g)].map((m) => m[1]).join('\n');
    const markup = source.replace(/<script[^>]*>[\s\S]*?<\/script>|<style[^>]*>[\s\S]*?<\/style>/g, '');
    for (const m of markup.matchAll(/\bstyle="([^"]*)"/g)) {
      const status = statusIn(declarations(m[1]).get('color') ?? '', carried);
      if (status) out.push({ path, selector: tidy(m[0]), status, plate: null });
    }
    // An expression is opaque to the tripwire: it counts when the component's script spells a status colour.
    const spelled = STATUS.exec(script)?.[1];
    for (const m of markup.matchAll(/\bstyle:color(?:\|important)?=(?:"([^"]*)"|\{([^}]*)\})/g)) {
      const status = m[1] !== undefined ? statusIn(m[1], carried) : (statusIn(m[2], carried) ?? (spelled && `--${spelled}`));
      if (status) out.push({ path, selector: tidy(m[0]), status, plate: null });
    }
  }
  return out;
}

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
    expect(bare).toEqual([]);
  });

  it('names no surface for a rule that is gone', () => {
    const live = new Set(loose.map((rule) => `${rule.path}: ${rule.selector}`));
    const stale = Object.entries(ON_SURFACE)
      .flatMap(([path, [, ...selectors]]) => selectors.map((selector) => `${path}: ${selector}`))
      .filter((entry) => !live.has(entry));
    expect(stale).toEqual([]);
  });

  it('keeps the surfaces it names inside a component opaque', () => {
    const open: string[] = [];
    for (const [path, selector] of Object.entries(LOCAL_SURFACE)) {
      expect(ON_SURFACE[path], path).toBeDefined();
      const css = styleOf(path).replace(/\/\*[\s\S]*?\*\//g, '');
      const rule = new RegExp(`(?:^|})\\s*${selector.replace('.', '\\.')}\\s*\\{([^}]*)\\}`).exec(css)?.[1];
      if (!plateOf(rule ?? '')) open.push(`${path}: ${selector}`);
    }
    expect(open).toEqual([]);
  });

  for (const id of ['dark', 'light', 'contrast', 'blush']) {
    it(`${id} keeps a status colour at 4.5:1 on every plate the pages give it`, () => {
      const t = presets().get(id) as Record<string, string>;
      expect(t, id).toBeDefined();
      const surfaces = [...new Set(paged.flatMap((rule) => (rule.plate && !rule.plate.tone ? [rule.plate.surface] : [])))];
      expect(surfaces.length).toBeGreaterThan(0);
      const failures: string[] = [];
      for (const surface of surfaces) {
        const on = fill(t, { surface });
        for (const name of ['--danger', '--warning', '--success']) {
          const r = ratio(rgb(t[name], on), on);
          if (r < 4.5) failures.push(`${name} on ${surface}: ${r.toFixed(2)}`);
        }
      }
      // A fill mixed from a tone is measured with the colour its own rule sets on it.
      for (const rule of paged) {
        if (!rule.plate?.tone) continue;
        const on = fill(t, rule.plate);
        const r = ratio(rgb(t[rule.status], on), on);
        if (r < 4.5) failures.push(`${rule.path}: ${rule.selector}: ${r.toFixed(2)}`);
      }
      expect(failures).toEqual([]);
    });
  }
});

// A badge sits wherever its row does, the bare page included (a ticket's status in the head of a
// ticket page): its label is text on its own fill, and that fill is opaque, so the label keeps its
// contrast over any backdrop.
const BADGES: Record<string, string[]> = {
  'lib/components/StatusBadge.svelte': ['.success', '.warning', '.danger', '.accent', '.neutral'],
  'lib/components/SystemRequirements.svelte': ['.badge', '.badge.success', '.badge.warning', '.badge.danger'],
};

describe('the fill of a badge', () => {
  const kinds = Object.entries(BADGES).flatMap(([path, selectors]) => {
    const css = styleOf(path).replace(/\/\*[\s\S]*?\*\//g, '');
    return selectors.map((selector) => {
      const body = new RegExp(`(?:^|})\\s*${selector.replace(/\./g, '\\.')}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? '';
      return { kind: `${path}: ${selector}`, plate: plateOf(body), label: /(?:^|[^-\w])color:\s*var\((--[\w-]+)\)/.exec(body)?.[1] };
    });
  });

  it('is opaque: a surface, or the tone mixed into one, never a translucent tint', () => {
    expect(kinds.filter((k) => !k.plate || !k.label).map((k) => k.kind)).toEqual([]);
  });

  for (const id of ['dark', 'light', 'contrast', 'blush']) {
    it(`${id} keeps a badge's label at 4.5:1 on its fill`, () => {
      const t = presets().get(id) as Record<string, string>;
      expect(t, id).toBeDefined();
      const failures: string[] = [];
      for (const { kind, plate, label } of kinds) {
        if (!plate || !label) continue;
        const on = fill(t, plate);
        const r = ratio(rgb(t[label], on), on);
        if (r < 4.5) failures.push(`${kind}: ${label} ${r.toFixed(2)}`);
      }
      expect(failures).toEqual([]);
    });
  }
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
        const r = ratio(rgb(t[label], on), on);
        if (r < 4.5) failures.push(`${name}: ${label} ${r.toFixed(2)}`);
      }
      expect(failures).toEqual([]);
    });
  }
});
