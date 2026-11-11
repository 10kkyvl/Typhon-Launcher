import { render } from 'svelte/server';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) }, Window: {} }));
vi.mock('../lib/services/backend', () => ({ inWails: false }));

import Downloads from './downloads/Downloads.svelte';
import Friends from './friends/Friends.svelte';
import Installed from './installed/Installed.svelte';
import Sources from './sources/Sources.svelte';
import { msg } from '../lib/i18n';
import { downloads } from '../lib/stores/downloads';
import { libraryGames } from '../lib/stores/library';
import { friendsPage } from '../lib/stores/social';
import { sources } from '../lib/stores/sources';
import { authState } from '../lib/stores/user';
import type { Download } from '../lib/services/downloads';
import type { LibraryGame } from '../lib/services/library';
import type { FriendView } from '../lib/services/social';
import type { Source } from '../lib/services/sources';

// A list of rows, each with its own menu button: a screen reader user moving through the buttons must
// hear whose menu each one opens, not the same "Menu" a dozen times.

// The accessible names of the menu buttons a page renders (they carry aria-haspopup="menu").
function menuButtonNames(html: string): string[] {
  return [...html.matchAll(/<button\b[^>]*aria-haspopup="menu"[^>]*>/g)].map((m) => /aria-label="([^"]*)"/.exec(m[0])?.[1] ?? '');
}

const unescape = (text: string) => text.replaceAll('&quot;', '"').replaceAll('&amp;', '&').replaceAll('&lt;', '<');

// One menu button per subject, its name the row's own message with the subject in it.
function expectNamed(names: string[], message: (subject: string) => string, subjects: string[]) {
  for (const subject of subjects) {
    const named = names.filter((name) => name.includes(subject));
    expect(named, `${subject} in ${JSON.stringify(names)}`).toEqual([message(subject)]);
  }
}

afterEach(() => {
  downloads.set([]);
  libraryGames.set([]);
  friendsPage.set({ friends: [], incoming: [], outgoing: [] });
  sources.set([]);
  authState.set('bootstrapping');
});

describe('row menu buttons name their row', () => {
  it('Downloads: each queued download', () => {
    downloads.set(
      ['Hollow Knight', 'Celeste'].map(
        (name, i) => ({ id: `q${i}`, name, status: 'queued', progress: 0, downloaded: 0, total: 1, files: [], origin: {} }) as unknown as Download,
      ),
    );

    const names = menuButtonNames(render(Downloads, { props: {} }).body).map(unescape);

    expectNamed(names, (name) => msg('transfers.downloadsQueueActionsFor', { name }), ['Hollow Knight', 'Celeste']);
  });

  it('Sources: each source', () => {
    sources.set(
      ['Main feed', 'Mirror'].map(
        (name, i) =>
          ({ id: `s${i}`, name, type: 'json', url: 'https://example.invalid/feed.json', enabled: true, status: 'active', health: 'healthy', lastUpdatedAt: null, lastError: '', fingerprint: '', feedVersion: 1, entries: 0, invalid: 0, matched: 0, review: 0, unmatched: 0, createdAt: '' }) as unknown as Source,
      ),
    );

    const names = menuButtonNames(render(Sources, { props: {} }).body).map(unescape);

    expectNamed(names, (name) => msg('transfers.sourcesMenuFor', { name }), ['Main feed', 'Mirror']);
  });

  it('Installed: each installed game', () => {
    libraryGames.set(
      ['Hollow Knight', 'Celeste'].map(
        (title, i) => ({ id: `g${i}`, title, installDir: `C:\\Games\\${i}`, executable: 'game.exe', sizeBytes: 0, lastPlayed: null, uninstalled: false, tags: [] }) as unknown as LibraryGame,
      ),
    );

    const names = menuButtonNames(render(Installed, { props: {} }).body).map(unescape);

    expectNamed(names, (title) => msg('games.installedMenuFor', { title }), ['Hollow Knight', 'Celeste']);
  });

  it('Friends: each friend', () => {
    authState.set('authenticated');
    friendsPage.set({
      friends: [
        { id: 'u1', username: 'neo', displayName: 'Neo', avatarUrl: '' },
        { id: 'u2', username: 'trin', displayName: '', avatarUrl: '' },
      ] as FriendView[],
      incoming: [],
      outgoing: [],
    });

    const names = menuButtonNames(render(Friends, { props: {} }).body).map(unescape);

    expectNamed(names, (name) => msg('social.friendMenuFor', { name }), ['Neo', 'trin']);
  });
});
