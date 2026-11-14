import { Heart } from '@lucide/svelte';
import { describe, expect, it } from 'vitest';
import type { GameCardAction } from './GameCard.svelte';

// Held by the type checker (npm run check); the run only keeps the file in the suite.
describe('GameCardAction', () => {
  it('is disabled only with a reason; a request that runs is busy', () => {
    const plain: GameCardAction = { id: 'a', label: 'A', icon: Heart };
    const busy: GameCardAction = { id: 'a', label: 'A', icon: Heart, busy: true };
    const unavailable: GameCardAction = { id: 'a', label: 'A', icon: Heart, disabled: true, reason: 'The favourites are full' };
    const flag = Math.random() > 2;
    const sometimes: GameCardAction = { id: 'a', label: 'A', icon: Heart, disabled: flag, reason: 'The favourites are full' };
    // @ts-expect-error: unavailable without saying why
    const silent: GameCardAction = { id: 'a', label: 'A', icon: Heart, disabled: true };
    // @ts-expect-error: a flag that may be on still needs its reason
    const unsaid: GameCardAction = { id: 'a', label: 'A', icon: Heart, disabled: flag };
    expect([plain, busy, unavailable, sometimes, silent, unsaid]).toHaveLength(6);
  });
});
