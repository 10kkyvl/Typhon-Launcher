import { describe, expect, it } from 'vitest';
import popular from './PopularGames.svelte?raw';
import activity from './Activity.svelte?raw';
import week from './WeekActivity.svelte?raw';
import stats from '../stats/Stats.svelte?raw';
import userStats from '../user/UserStats.svelte?raw';

/**
 * The statistics, activity and user-statistics screens write type sizes and spacing with the scale's
 * tokens, and draw a figure with StatTile and a section's heading with SectionHeader, not a local copy.
 * The column widths of the activity layout and the cover sizes are the layout's own, not spacing.
 * A source tripwire: what the page looks like is shot in the live app.
 */
const FILES: Record<string, string> = {
  'routes/activity/Activity.svelte': activity,
  'routes/activity/PopularGames.svelte': popular,
  'routes/activity/WeekActivity.svelte': week,
  'routes/stats/Stats.svelte': stats,
  'routes/user/UserStats.svelte': userStats,
};

const style = (source: string) => /<style>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '';

/** Every `property: value` of a style block whose property is in `names`. */
function declarations(css: string, names: RegExp): string[] {
  return [...css.matchAll(/^\s*([a-z-]+):\s*([^;]+);/gm)].filter(([, name]) => names.test(name)).map(([, name, value]) => `${name}: ${value.trim()}`);
}

/** A rule naming a figure's or a heading's class anywhere in its selector, or any :global reach-in. */
const COPY_OR_REACH_IN = [/(^|[\s,>+~(])\.(eyebrow|longest|tile)\b[^{;]*\{/m, /:global\b/];

const SPACING = /^(gap|row-gap|column-gap|margin(-\w+)?|padding(-\w+)?)$/;
const SPACE_VALUE = /^((var\(--space-\d+\)|0|auto|calc\(var\(--space-\d+\) \* -1\))\s*)+$/;

describe('the statistics and activity styles', () => {
  it('write font sizes with the type scale', () => {
    const literal = Object.entries(FILES).flatMap(([path, source]) =>
      declarations(style(source), /^font-size$/)
        .filter((line) => !/^font-size: var\(--font-[a-z]+\)$/.test(line))
        .map((line) => `${path}: ${line}`),
    );
    expect(literal).toEqual([]);
  });

  it('write gaps, margins and paddings with the spacing scale', () => {
    const literal = Object.entries(FILES).flatMap(([path, source]) =>
      declarations(style(source), SPACING)
        .filter((line) => !SPACE_VALUE.test(line.slice(line.indexOf(':') + 1).trim()))
        .map((line) => `${path}: ${line}`),
    );
    expect(literal).toEqual([]);
  });

  it('draw no local copy of a figure or of a section heading, and reach into no base part', () => {
    const markup = (source: string) => source.replace(/<(script|style)\b[\s\S]*?<\/\1>/g, '');
    const copies = Object.entries(FILES).flatMap(([path, source]) => [
      ...COPY_OR_REACH_IN.filter((pattern) => pattern.test(style(source))).map((pattern) => `${path}: ${pattern.source}`),
      ...[/class="(value|tile|eyebrow)"/, /<h[2-6]\b(?![^>]*class="name")/].filter((pattern) => pattern.test(markup(source))).map((pattern) => `${path}: ${pattern.source}`),
    ]);
    expect(copies).toEqual([]);
  });

  it('reads what it guards', () => {
    expect(declarations('a {\n  gap: 0.4rem;\n  font-size: 1.2rem;\n}', SPACING)).toEqual(['gap: 0.4rem']);
    expect(SPACE_VALUE.test('var(--space-4) calc(var(--space-4) * -1) 0')).toBe(true);
    expect(SPACE_VALUE.test('0.6rem var(--space-3)')).toBe(false);
    const caught = (css: string) => COPY_OR_REACH_IN.some((pattern) => pattern.test(css));
    expect(caught('  .tile {\n    gap: 0;\n  }')).toBe(true);
    expect(caught('  .stats :global(.tile) {\n    align-items: center;\n  }')).toBe(true);
    expect(caught('  .stats .tile,\n  .row {\n    gap: 0;\n  }')).toBe(true);
    expect(caught('  .stats {\n    text-align: center;\n  }\n  .tiles {\n    gap: 0;\n  }')).toBe(false);
  });
});
