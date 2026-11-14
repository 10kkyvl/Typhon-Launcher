import { tick } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { locale } from '../../i18n';
import type { LibraryGame } from '../../services/library';
import { libraryGames } from '../../stores/library';
import { renderDom } from '../../testing/dom';
import LibraryPage from './LibraryPage.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../testing/dom')).wailsRuntime());
vi.mock('../../services/backend', () => ({ inWails: false }));

function game(id: string, title: string): LibraryGame {
  return { id, title, executable: 'game.exe', installDir: `C:\\Games\\${id}`, cover: '', version: '', sizeBytes: 0, lastPlayed: null, playtimeSeconds: 0, installedAt: '' };
}

const titles = () => [...document.querySelectorAll('.library-card strong')].map((el) => el.textContent?.trim());

afterEach(() => {
  libraryGames.set([]);
  document.body.replaceChildren();
});

describe('Big Picture library page', () => {
  it('orders titles in the interface language, also after it changes while the page is open', async () => {
    libraryGames.set([game('w', 'Witcher'), game('v', 'Ведьмак')]);
    locale.set('ru');
    renderDom(LibraryPage, {
      onback: () => {},
      ongame: () => {},
      oncatalog: () => {},
      onlaunch: async () => false,
      requestText: async () => null,
    });
    expect(titles()).toEqual(['Ведьмак', 'Witcher']);

    locale.set('en');
    await tick();

    expect(titles()).toEqual(['Witcher', 'Ведьмак']);
  });
});
