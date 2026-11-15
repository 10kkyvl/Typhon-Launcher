import { describe, expect, it } from 'vitest';
import { frontendSources } from '../../lib/testing/sources';

// The game page claims the window backdrop and the stage for its game. The claims are wiring in the page
// component, so the source is pinned: the page claims the game pageAmbient picks for it, only while it
// shows a game, and releases each claim as its effect's cleanup. GameDetails.stage.dom.test.ts renders it.
describe('the game page, the window backdrop and the stage', () => {
  const source = frontendSources['routes/game/GameDetails.svelte'];

  it('claims the game pageAmbient picks, only while the page shows a game', () => {
    const derived = /const (\w+) = \$derived\(pageAmbient\(localGame\?\.id, canonicalId, (\w+)\)\);/.exec(source);
    expect(derived, 'the claimed id comes from pageAmbient').not.toBeNull();
    const [, claimed, shown] = derived as RegExpExecArray;
    expect(source).toMatch(new RegExp(`\\$effect\\(\\(\\) => claimAmbient\\(${claimed}\\)\\);`));
    expect(source).toMatch(new RegExp(`const ${shown} = \\$derived\\(!missing && !loadFailed\\);`));
    expect(source).toMatch(new RegExp(`<Page bleed=\\{${shown} \\? hero : undefined\\}>`));
  });

  it('claims the stage for the same game as the backdrop and asks the stage about that game', () => {
    const [, claimed] = /const (\w+) = \$derived\(pageAmbient\(/.exec(source) as RegExpExecArray;
    expect(source).toMatch(new RegExp(`\\$effect\\(\\(\\) => claimStage\\(${claimed}\\)\\);`));
    expect(source).toMatch(new RegExp(`stageWide\\(${claimed}\\)`));
  });
});
