import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { LibraryGame } from '../services/library';
import { libraryGames } from '../stores/library';
import { user } from '../testing/dom';
import RandomGameModal from './RandomGameModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

// The spin is held by the test: it ends when the test says so.
let finish: (() => void) | undefined;
vi.mock('../game/randomPick', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../game/randomPick')>()),
  reducedMotion: () => false,
  runSpin: (frames: LibraryGame[], onFrame: (game: LibraryGame) => void, onDone: (game: LibraryGame) => void) => {
    onFrame(frames[0]);
    finish = () => onDone(frames[frames.length - 1]);
    return () => {};
  },
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([{ id: 'g1', title: 'Hades', uninstalled: false, installDir: '/games/hades', executable: 'Hades.exe' } as LibraryGame]);
});

afterEach(() => {
  libraryGames.set([]);
  finish = undefined;
  document.body.replaceChildren();
});

describe('RandomGameModal: spinning from the keyboard', () => {
  it('keeps the focus on the pressed Spin, busy, while it spins, then hands it to "Again"', async () => {
    render(RandomGameModal, { target: app, props: { onclose: vi.fn() } });
    const dialog = screen.getByRole('dialog', { name: msg('games.randomTitle') });
    const keys = user();
    const spin = within(dialog).getByRole('button', { name: msg('games.randomSpin') });
    spin.focus();

    await keys.keyboard('{Enter}');

    expect(spin.isConnected).toBe(true);
    expect(spin.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(spin);

    finish?.();
    const again = await within(dialog).findByRole('button', { name: msg('games.randomAgain') });
    await vi.waitFor(() => expect(document.activeElement).toBe(again));
  });

  it('keeps the focus on the pressed "Again", busy, while it spins and after', async () => {
    render(RandomGameModal, { target: app, props: { onclose: vi.fn() } });
    const dialog = screen.getByRole('dialog', { name: msg('games.randomTitle') });
    const keys = user();
    await keys.click(within(dialog).getByRole('button', { name: msg('games.randomSpin') }));
    finish?.();
    const again = await within(dialog).findByRole('button', { name: msg('games.randomAgain') });
    again.focus();

    await keys.keyboard('{Enter}');

    expect(again.isConnected).toBe(true);
    expect(again.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(again);

    finish?.();
    await vi.waitFor(() => expect(again.getAttribute('aria-busy')).toBeNull());
    expect(document.activeElement).toBe(again);
  });

  it('leaves the focus alone when the pressed Spin did not hold it', async () => {
    render(RandomGameModal, { target: app, props: { onclose: vi.fn() } });
    const dialog = screen.getByRole('dialog', { name: msg('games.randomTitle') });
    (document.activeElement as HTMLElement | null)?.blur();
    expect(document.activeElement).toBe(document.body);

    within(dialog).getByRole('button', { name: msg('games.randomSpin') }).click();
    finish?.();

    await within(dialog).findByRole('button', { name: msg('games.randomAgain') });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(document.activeElement).toBe(document.body);
  });
});
