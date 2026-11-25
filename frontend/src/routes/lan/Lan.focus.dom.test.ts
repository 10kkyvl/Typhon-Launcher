import { screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/lan', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/lan')>()),
  getOffers: vi.fn(async () => get((await import('../../lib/stores/lan')).offers)),
  cancel: vi.fn(),
}));

import { msg } from '../../lib/i18n';
import { cancel } from '../../lib/services/lan';
import type { LibraryGame } from '../../lib/services/library';
import { hashing, shares, transfers } from '../../lib/stores/lan';
import { libraryGames } from '../../lib/stores/library';
import { renderDom, user } from '../../lib/testing/dom';
import Lan from './Lan.svelte';
import { progress, ready, resetScreen, share, transfer } from './lanTesting';

beforeEach(() => {
  ready();
  Element.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  resetScreen();
  vi.mocked(cancel).mockReset();
});

const game = (id: string, title: string) => ({ id, title }) as LibraryGame;
const stop = (title: string) => screen.getByRole('button', { name: msg('transfers.lanStopSharingFor', { title }) });
const cancelOf = (title: string) => screen.getByRole('button', { name: msg('transfers.lanCancelFor', { title }) });
const cancelDialog = (title: string) => screen.queryByRole('dialog', { name: msg('transfers.lanCancelTitle', { title }) });
const pageHeading = () => screen.getByRole('heading', { level: 1 });

describe('Lan: the steps of the focus rule', () => {
  it('the focused row and the one above it leave together: focus goes to the same control of the row above them, not to the section’s first control', async () => {
    // Hades is in the library: its title is a button, the section's first control.
    libraryGames.set([game('g1', 'Hades')]);
    shares.set([share(), share({ gameId: 'g2', title: 'Inside' }), share({ gameId: 'g3', title: 'Limbo' })]);
    renderDom(Lan);
    stop('Limbo').focus();

    shares.set([share()]);

    await waitFor(() => expect(document.activeElement).toBe(stop('Hades')));
  });

  it('the only row with a control leaves beside a row with none: focus goes to the page heading, never to the page body', async () => {
    shares.set([share()]);
    // A game still in preparation, not in the library: its row holds no control.
    hashing.set(new Map([['g9', progress('g9', 50)]]));
    renderDom(Lan);
    stop('Hades').focus();

    shares.set([]);

    await waitFor(() => expect(document.activeElement).toBe(pageHeading()));
  });

  it('the only row with a control leaves beside one whose title opens its game: focus goes to that title', async () => {
    libraryGames.set([game('g9', 'Celeste')]);
    shares.set([share()]);
    hashing.set(new Map([['g9', progress('g9', 50)]]));
    renderDom(Lan);
    stop('Hades').focus();

    shares.set([]);

    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Celeste' })));
  });
});

describe('Lan: a transfer that ends while its cancel is asked', () => {
  it('the question closes by itself, and focus goes by the row rule to the next transfer’s Cancel', async () => {
    transfers.set([transfer({ downloaded: 10 }), transfer({ id: 't2', title: 'Inside', downloaded: 10 })]);
    renderDom(Lan);
    const keys = user();
    cancelOf('Celeste').focus();
    await keys.keyboard('{Enter}');
    expect(cancelDialog('Celeste')).not.toBeNull();

    transfers.set([transfer({ id: 't2', title: 'Inside', downloaded: 10 })]);

    await waitFor(() => expect(cancelDialog('Celeste')).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(cancelOf('Inside')));
    expect(cancel).not.toHaveBeenCalled();
  });

  it('the last transfer ends while asked: the question closes and focus goes to the page heading', async () => {
    transfers.set([transfer({ downloaded: 10 })]);
    renderDom(Lan);
    const keys = user();
    cancelOf('Celeste').focus();
    await keys.keyboard('{Enter}');
    const dialog = cancelDialog('Celeste') as HTMLElement;
    expect(within(dialog).getByRole('button', { name: msg('transfers.lanCancelConfirm') })).toBeTruthy();

    transfers.set([]);

    await waitFor(() => expect(cancelDialog('Celeste')).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(pageHeading()));
  });
});
