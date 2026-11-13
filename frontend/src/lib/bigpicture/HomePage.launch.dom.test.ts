import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { LibraryGame } from '../services/library';
import { libraryGames } from '../stores/library';
import { user } from '../testing/dom';
import HomePage from './components/HomePage.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => (resolve = res));
  return { promise, resolve };
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  document.body.append(app);
  libraryGames.set([{ id: 'g1', title: 'Hades', uninstalled: false, installDir: '/games/hades', executable: 'Hades.exe', lastPlayed: '2026-11-01T10:00:00Z', playtimeSeconds: 0 } as LibraryGame]);
});

afterEach(() => {
  app.remove();
  libraryGames.set([]);
});

const playButton = () => document.querySelector<HTMLButtonElement>('[data-bp-focus="home-play"]');

// The launch Big Picture hands the page waits while a question (the save conflict) is open.
describe('Big Picture home page: a launch that waits on a question', () => {
  it('shows no error while the launch waits, and after it was given up Play works again', async () => {
    const asked = deferred<boolean>();
    const onlaunch = vi.fn().mockReturnValueOnce(asked.promise).mockReturnValueOnce(new Promise(() => {}));
    render(HomePage, { target: app, props: { ongame: vi.fn(), oncatalog: vi.fn(), onlaunch } });
    const keys = user();
    await vi.waitFor(() => expect(playButton()).not.toBeNull());

    await keys.click(playButton()!);
    expect(onlaunch).toHaveBeenCalledWith('g1');
    expect(screen.queryByRole('alert')).toBeNull();

    asked.resolve(false);
    await vi.waitFor(() => expect(playButton()).toHaveProperty('disabled', false));
    expect(screen.queryByRole('alert')).toBeNull();

    await keys.click(playButton()!);
    expect(onlaunch).toHaveBeenCalledTimes(2);
  });

  it('shows a conflict that held the launch back again in its own words', async () => {
    const onlaunch = vi.fn(async () => {
      throw new Error('typhon:cloud.conflict_unresolved: diverged');
    });
    render(HomePage, { target: app, props: { ongame: vi.fn(), oncatalog: vi.fn(), onlaunch } });
    await vi.waitFor(() => expect(playButton()).not.toBeNull());

    await user().click(playButton()!);

    expect((await screen.findByRole('alert')).textContent?.trim()).toBe(msg('cloud.conflict_unresolved'));
  });
});
