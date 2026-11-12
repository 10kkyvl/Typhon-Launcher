import { screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import { renderDom } from '../testing/dom';
import SearchInput from './SearchInput.svelte';

describe('SearchInput', () => {
  it('is named by its label; the placeholder stays an example', () => {
    renderDom(SearchInput, { label: 'Username or friend code', placeholder: '@username or code TY-XXXX-XXXX' });
    const field = screen.getByRole('textbox', { name: 'Username or friend code' });
    expect(field.getAttribute('placeholder')).toBe('@username or code TY-XXXX-XXXX');
  });

  it('is named by a visible label elsewhere on the page', () => {
    const heading = document.createElement('h4');
    heading.id = 'manual-search';
    heading.textContent = 'Search by name';
    document.body.append(heading);
    renderDom(SearchInput, { labelledby: 'manual-search', placeholder: 'Game name' });
    expect(screen.getByRole('textbox', { name: 'Search by name' })).toBeTruthy();
    heading.remove();
  });
});
