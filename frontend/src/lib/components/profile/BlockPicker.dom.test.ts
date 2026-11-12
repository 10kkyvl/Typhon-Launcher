import { screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';

vi.mock('../../services/backend', () => ({ inWails: false }));

import { msg } from '../../i18n';
import { renderDom } from '../../testing/dom';
import BlockPicker from './BlockPicker.svelte';

describe('BlockPicker', () => {
  it('its button says that it shows the list of blocks below it, and which list, not that it is pressed', async () => {
    const { user } = renderDom(BlockPicker, { layout: { version: 1, blocks: [] }, onadd: vi.fn() });
    const add = screen.getByRole('button', { name: msg('profile.addBlock') });
    expect(add.getAttribute('aria-expanded')).toBe('false');
    expect(add.hasAttribute('aria-pressed')).toBe(false);

    await user.click(add);

    expect(add.getAttribute('aria-expanded')).toBe('true');
    const list = document.getElementById(add.getAttribute('aria-controls') ?? '');
    expect(list?.tagName).toBe('UL');
    expect(list?.querySelectorAll('button').length).toBeGreaterThan(0);

    await user.click(add);

    expect(add.getAttribute('aria-expanded')).toBe('false');
  });
});
