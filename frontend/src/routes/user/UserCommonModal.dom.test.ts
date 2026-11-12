import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { userCommon, type CommonGame, type CommonGamesPage } from '../../lib/services/social';
import { user } from '../../lib/testing/dom';
import UserCommonModal from './UserCommonModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/social', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/social')>()),
  userCommon: vi.fn(),
}));

const game = (igdbId: number, title: string): CommonGame => ({ igdbId, title, coverUrl: '', viewerOwned: true, targetOwned: true });

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  vi.mocked(userCommon).mockReset();
  document.body.replaceChildren();
});

describe('UserCommonModal: loading more', () => {
  it('keeps the focus on "load more", busy, while the next page loads', async () => {
    let answer!: (page: CommonGamesPage) => void;
    vi.mocked(userCommon)
      .mockResolvedValueOnce({ games: [game(1, 'Hades')], next: 'c1' })
      .mockReturnValueOnce(new Promise<CommonGamesPage>((resolve) => (answer = resolve)));
    render(UserCommonModal, { target: app, props: { open: true, username: 'lena', name: 'Lena' } });
    const dialog = screen.getByRole('dialog', { name: msg('social.userCommonAllTitle') });
    const keys = user();
    const more = await within(dialog).findByRole('button', { name: msg('social.userCommonMore') });
    more.focus();

    await keys.keyboard('{Enter}');

    expect(more.getAttribute('aria-busy')).toBe('true');
    expect(more).toHaveProperty('disabled', false);
    expect(document.activeElement).toBe(more);
    answer({ games: [game(2, 'Celeste')], next: '' });
    await within(dialog).findByText('Celeste');
  });
});
