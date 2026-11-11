import { describe, expect, it } from 'vitest';
import { renderDom } from '../testing/dom';
import Card from './Card.svelte';

describe('Card in a DOM', () => {
  it('takes its padding as a named size, the default one when none is given', () => {
    const plain = renderDom(Card, {});
    const card = plain.container.querySelector<HTMLElement>('.card');
    expect(card?.classList.contains('pad-md')).toBe(true);
    expect(card?.getAttribute('style')).toBeNull();

    const small = renderDom(Card, { padding: 'sm-even' });
    expect(small.container.querySelector('.card')?.classList.contains('pad-sm-even')).toBe(true);
  });

  it('shows its title as a heading and is a panel when asked', () => {
    const { getByRole, container } = renderDom(Card, { title: 'About', surface: 'panel' });
    expect(getByRole('heading', { level: 3, name: 'About' })).toBeTruthy();
    expect(container.querySelector('.card')?.classList.contains('panel')).toBe(true);
  });
});
