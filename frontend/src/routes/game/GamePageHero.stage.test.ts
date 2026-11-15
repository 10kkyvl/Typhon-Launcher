import { describe, expect, it } from 'vitest';
import { frontendSources } from '../../lib/testing/sources';

// The plate keeps AA for --text alone (theme-contrast.test.ts, .stage-copy); --text-2 is --text there.
// A part of the header in the quietest grey is raised to --text on the stage, or it would read below AA.
describe('the game page header on the stage', () => {
  it('raises every part drawn in the quietest grey to the one text colour on the stage', () => {
    const css = (frontendSources['routes/game/GamePageHero.svelte'].split('<style>')[1] ?? '').replace(/\/\*[\s\S]*?\*\//g, '');
    const quiet = [...css.matchAll(/([^{}]+)\{[^}]*color:\s*var\(--text-3\)/g)].flatMap(([, selectors]) => selectors.split(',').map((s) => s.trim()));
    expect(quiet.length).toBeGreaterThan(0);
    const raised = /\.stage-copy :is\(([^)]*)\)\s*\{\s*color:\s*var\(--text\);\s*\}/.exec(css)?.[1].split(',').map((s) => s.trim()) ?? [];
    expect(quiet.filter((selector) => !raised.includes(selector))).toEqual([]);
  });
});
