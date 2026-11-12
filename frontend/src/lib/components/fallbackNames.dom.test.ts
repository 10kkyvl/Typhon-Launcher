import { screen, within } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import { renderDom } from '../testing/dom';
import FallbackNamesHarness from '../testing/FallbackNamesHarness.svelte';

// With no picture to show, a cover or an avatar draws the first letters of the name. They are
// decoration: read out, they put a stray letter before the name of whatever holds them.
describe('the letters drawn in place of a missing picture', () => {
  it('stay out of the name of a row that is named by its text', () => {
    renderDom(FallbackNamesHarness);
    expect(screen.getByRole('button', { name: 'Celeste (2018)' })).toBe(screen.getByTestId('game-row'));
    expect(screen.getByRole('button', { name: 'Ann Lee' })).toBe(screen.getByTestId('friend-row'));
  });

  it('are still drawn', () => {
    renderDom(FallbackNamesHarness);
    expect(screen.getByTestId('game-row').textContent).toContain('C');
    expect(screen.getByTestId('bare').textContent?.trim()).toBe('HK');
    expect(screen.getByTestId('friend-row').querySelector('.fallback')?.textContent).toBe('A');
  });

  it('a cover that alone names its control still names it, with a picture or without', () => {
    renderDom(FallbackNamesHarness);
    expect(screen.getByRole('button', { name: 'Celeste' })).toBe(screen.getByTestId('cover-only'));
    expect(within(screen.getByTestId('cover-only')).getByRole('img', { name: 'Celeste' })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Hades' })).toBe(screen.getByTestId('cover-link'));
  });

  it('a cover with no text alternative shows nothing to assistive technology', () => {
    renderDom(FallbackNamesHarness);
    const bare = screen.getByTestId('bare');
    expect(within(bare).queryByRole('img')).toBeNull();
    expect([...bare.querySelectorAll('*')].filter((el) => el.children.length === 0 && !el.closest('[aria-hidden="true"]')).map((el) => el.textContent)).toEqual([]);
  });

  it('a control named by its own label keeps that name around an avatar', () => {
    renderDom(FallbackNamesHarness);
    expect(screen.getByRole('button', { name: 'Profile of Ann Lee' })).toBe(screen.getByTestId('friend-labelled'));
  });
});
