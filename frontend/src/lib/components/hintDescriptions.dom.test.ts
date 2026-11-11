import { render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { PopularGame } from '../social/popular';
import type { ReleaseView } from '../services/sources';
import { statusReason } from '../services/selfupdateMessages';
import { autoAway } from '../stores/presence';
import { selfUpdateStatus } from '../stores/selfupdate';
import { authState, currentUser } from '../stores/user';
import { description, hintOnFocus, tabbable } from '../testing/hints';
import FriendCodeCard from '../../routes/friends/FriendCodeCard.svelte';
import PopularGames from '../../routes/activity/PopularGames.svelte';
import TokenValueInput from '../../routes/settings/TokenValueInput.svelte';
import AccountMenu from './AccountMenu.svelte';
import ReleaseMatchModal from './ReleaseMatchModal.svelte';
import TagEditor from './TagEditor.svelte';
import SidebarStatus from './SidebarStatus.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/sources')>()),
  getMatchCandidates: vi.fn(async () => [
    { gameId: 'g1', title: 'Hades', score: 0.9, method: 'title', source: 'local', inCatalog: false },
    { gameId: 'g2', title: 'Hades II', score: 0.8, method: 'title', source: 'local' },
  ]),
}));

// A hint that says more than a control's name (class c): the control keeps its name, the hint is its
// description, keyboard focus shows it, and the control has no native title any more.
async function expectDescribed(control: HTMLElement, text: string) {
  expect(control.hasAttribute('title')).toBe(false);
  expect(tabbable(control)).toBe(true);
  expect(description(control)).toBe(text);
  expect(await hintOnFocus(control)).toBe(text);
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
  selfUpdateStatus.set({ state: 'idle', currentVersion: '' });
  autoAway.set(false);
  currentUser.set(null);
  authState.set('bootstrapping');
});

describe('hints that describe a control', () => {
  it('the sidebar update states: what a download or a restart is about, and why an update failed', async () => {
    selfUpdateStatus.set({ state: 'available', currentVersion: '1.4.1', availableVersion: '1.5.0' });
    const view = render(SidebarStatus, { target: app });
    await expectDescribed(
      screen.getByRole('button', { name: msg('ui.statusUpdateDownload', { version: '1.5.0' }) }),
      msg('ui.versionAvailable', { version: '1.5.0' }),
    );
    view.unmount();

    selfUpdateStatus.set({ state: 'ready', currentVersion: '1.4.1', availableVersion: '1.5.0' });
    const ready = render(SidebarStatus, { target: app });
    await expectDescribed(screen.getByRole('button', { name: msg('ui.restartAndUpdate') }), msg('ui.updateReadyMessage', { version: '1.5.0' }));
    ready.unmount();

    const failed = { state: 'available', currentVersion: '1.4.1', availableVersion: '1.5.0', error: 'x', errorCode: 'download' } as const;
    selfUpdateStatus.set(failed);
    render(SidebarStatus, { target: app });
    await expectDescribed(screen.getByRole('button', { name: msg('ui.statusUpdateRetry', { version: '1.5.0' }) }), statusReason(failed));
  });

  it('the account chip: why the status went away by itself', async () => {
    authState.set('authenticated');
    currentUser.set({ id: 'me', username: 'me', displayName: 'Me', avatarUrl: '' } as never);
    autoAway.set(true);
    render(AccountMenu, { target: app });
    const chip = screen.getByRole('button', { name: /Me/ });
    await expectDescribed(chip, msg('ui.presenceAutoAwayHint'));
  });

  it('popular games: which friends play it', async () => {
    const games = [{ game: { igdbId: 1, title: 'Hades', coverUrl: '' }, count: 2, playing: 0, names: ['Ann', 'Bob'] }] as unknown as PopularGame[];
    render(PopularGames, { target: app, props: { games } });
    await expectDescribed(screen.getByRole('button', { name: /Hades/ }), 'Ann, Bob');
  });

  it('the friend code: pressing it copies', async () => {
    render(FriendCodeCard, { target: app, props: { code: 'ABCD-1234' } });
    await expectDescribed(screen.getByRole('button', { name: 'ABCD-1234' }), msg('social.friendsCopyCode'));
  });

  it('a theme token field: its CSS name, next to the range error the field gains', async () => {
    const { rerender } = render(TokenValueInput, { target: app, props: { id: 'f', name: '--radius-sm', value: '6px', onchange: () => {} } });
    const field = screen.getByRole('textbox');
    await expectDescribed(field, '--radius-sm');
    await rerender({ id: 'f', name: '--radius-sm', value: '6px', describedBy: 'f-hint', onchange: () => {} });
    expect(field.getAttribute('aria-describedby')?.split(' ')).toContain('f-hint');
  });

  it('a release candidate missing from the catalog: why it cannot be picked, and it is not picked', async () => {
    const release = { release: { id: 'r1', rawTitle: 'Hades', normalizedTitle: 'hades', size: 1, uploadedAt: '' }, sourceName: 'Feed', gameTitle: '' } as unknown as ReleaseView;
    render(ReleaseMatchModal, { target: app, props: { open: true, release } });
    const missing = await waitFor(() => screen.getByRole('button', { name: /^Hades\b(?! II)/ }));
    await expectDescribed(missing, msg('modals.releaseMatchNotInCatalog'));
    expect(missing.getAttribute('aria-disabled')).toBe('true');
    missing.click();
    expect(missing.classList.contains('selected')).toBe(false);
  });

  it('a tag chip: pressing it removes the tag', async () => {
    render(TagEditor, { target: app, props: { tags: ['coop'], placeholder: 'Tag' } });
    await expectDescribed(screen.getByRole('button', { name: /coop/ }), msg('modals.gameTagsRemove', { tag: 'coop' }));
  });
});
