import { describe, expect, it } from 'vitest';
import { ruleBody, styleOf } from '../testing/css';
import source from './GameCard.svelte?raw';

// A source tripwire for what jsdom cannot lay out; the live run in Chromium is the proof.
const css = styleOf(source, 'GameCard.svelte');

describe('GameCard: the compat verdict on the cover', () => {
  it('stays inside the card, whatever the length of its label', () => {
    const box = ruleBody(css, '.compat');
    expect(box['max-width']).toBe('calc(100% - 1.2rem)');
    expect(box.overflow).toBe('hidden');
  });

  it('steps aside wherever the play button shows over the same corner', () => {
    expect(ruleBody(css, '.cover-wrap.playable:hover .compat,\n  .cover-wrap.playable:focus-within .compat,\n  .cover-wrap.running .compat')).toEqual({ opacity: '0' });
    expect(ruleBody(css, '.cover-wrap:hover .play,\n  .play:focus-visible,\n  .play.running').opacity).toBe('1');
  });
});
