import { describe, expect, it } from 'vitest';
import { frontendSources } from '../../lib/testing/sources';

// The game page is the one screen that claims the window backdrop for its game. The claim is wiring in the
// page component that no render test reaches, so the source is pinned: the page claims the game
// pageAmbient picks for it, only while it shows a game, and releases the claim as the effect's cleanup.
describe('the game page and the window backdrop', () => {
  const source = frontendSources['routes/game/GameDetails.svelte'];

  it('claims the game pageAmbient picks, only while the page shows a game', () => {
    const derived = /const (\w+) = \$derived\(pageAmbient\(localGame\?\.id, canonicalId, (\w+)\)\);/.exec(source);
    expect(derived, 'the claimed id comes from pageAmbient').not.toBeNull();
    const [, claimed, shown] = derived as RegExpExecArray;
    expect(source).toMatch(new RegExp(`\\$effect\\(\\(\\) => claimAmbient\\(${claimed}\\)\\);`));
    expect(source).toMatch(new RegExp(`const ${shown} = \\$derived\\(!missing && !loadFailed\\);`));
    expect(source).toMatch(new RegExp(`<Page bleed=\\{${shown} \\? hero : undefined\\}>`));
  });
});
