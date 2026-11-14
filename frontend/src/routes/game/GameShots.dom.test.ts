import { screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { MediaAsset } from '../../lib/services/metadata';
import { renderDom, user } from '../../lib/testing/dom';
import GameShots from './GameShots.svelte';

const shot = (id: string): MediaAsset => ({ id, url: `https://img/${id}.jpg` }) as MediaAsset;

describe('GameShots', () => {
  it('names each picture with its place among them and opens the one pressed', async () => {
    const onopen = vi.fn();
    renderDom(GameShots, { shots: [shot('a'), shot('b'), shot('c')], onopen });

    const names = screen.getAllByRole('button').map((button) => button.getAttribute('aria-label'));
    expect(names).toEqual([1, 2, 3].map((index) => msg('games.detailOpenScreenshotAt', { index, count: 3 })));
    expect(new Set(names).size).toBe(3);

    await user().click(screen.getByRole('button', { name: msg('games.detailOpenScreenshotAt', { index: 2, count: 3 }) }));
    expect(onopen).toHaveBeenCalledWith(1);
  });
});
