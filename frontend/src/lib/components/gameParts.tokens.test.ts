import { describe, expect, it } from 'vitest';
import { declarations, styleOf } from '../testing/css';
import { frontendSources } from '../testing/sources';

/**
 * The parts a game is shown with (its card, its picture, the library row and the hero carousel) take
 * every colour and layer from tokens, as the base controls do (styles/control-tokens.test.ts): what is
 * drawn over a picture comes from the fixed --art-* tokens of shell.css, which theme-contrast.test.ts
 * measures over black and white.
 */

const PARTS = ['lib/components/GameCard.svelte', 'lib/components/Artwork.svelte', 'lib/art/GeneratedArt.svelte', 'lib/components/GameListRow.svelte'];
const COLOUR = /#[0-9a-f]{3,8}\b|\b(?:rgba?|hsla?|oklch|oklab|lab|lch)\(|\b(?:white|black)\b/i;

describe('the game parts take their colours and layers from tokens', () => {
  for (const path of PARTS) {
    const source = frontendSources[path];

    it(`${path} writes no colour and sets no layer`, () => {
      expect(source, path).toBeTypeOf('string');
      const all = declarations(styleOf(source, path));
      expect(all.length).toBeGreaterThan(0);
      expect(all.filter((d) => COLOUR.test(d.value)).map((d) => `${d.prop}: ${d.value}`)).toEqual([]);
      expect(all.filter((d) => d.prop === 'z-index').map((d) => `${d.prop}: ${d.value}`)).toEqual([]);
      expect(/@keyframes\b/.test(source)).toBe(false);
    });
  }
});
