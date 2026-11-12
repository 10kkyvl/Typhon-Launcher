import { screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { renderDom } from '../testing/dom';
import ReferenceLinks from './ReferenceLinks.svelte';

describe('ReferenceLinks: a reference site', () => {
  // The page has no address here: the launcher builds it and opens it, so the chip is a button.
  it('is a chip button with the external mark out of its name, opened by the launcher', async () => {
    const onopen = vi.fn();
    const { user } = renderDom(ReferenceLinks, { kinds: ['pcgamingwiki'], onopen });
    const chip = screen.getByRole('button', { name: msg('games.detailReferencePcgw') });
    expect(chip.classList.contains('chip')).toBe(true);
    expect(chip.querySelector('.end')?.getAttribute('aria-hidden')).toBe('true');
    chip.focus();

    await user.keyboard('{Enter}');

    expect(onopen).toHaveBeenCalledWith('pcgamingwiki');
  });
});
