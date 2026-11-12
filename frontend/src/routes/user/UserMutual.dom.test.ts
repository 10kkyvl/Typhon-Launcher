import { screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { resetHistory, route } from '../../lib/stores/router';
import { renderDom } from '../../lib/testing/dom';
import UserMutual from './UserMutual.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());

afterEach(() => {
  resetHistory();
});

describe('UserMutual', () => {
  it('leads to all friends through a text link the keyboard presses', async () => {
    const friends = [{ id: 'u1', username: 'ann', displayName: 'Ann', avatarUrl: '' }];
    const { user } = renderDom(UserMutual, { friends: friends as never, count: 1 });
    const all = screen.getByRole('button', { name: msg('social.viewAllButton') });
    expect(all.classList.contains('text-link')).toBe(true);
    all.focus();

    await user.keyboard('{Enter}');

    expect(get(route).name).toBe('friends');
  });
});
