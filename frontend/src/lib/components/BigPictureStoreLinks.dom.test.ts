import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { user } from '../testing/dom';
import BigPictureShell from './BigPictureShell.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../bigpicture/components/HomePage.svelte', async () => ({ default: (await import('../testing/BigPictureStorePageStub.svelte')).default }));
vi.mock('../bigpicture/components/LibraryPage.svelte', async () => ({ default: (await import('../testing/BigPictureStorePageStub.svelte')).default }));
vi.mock('../bigpicture/components/CatalogPage.svelte', async () => ({ default: (await import('../testing/BigPictureStorePageStub.svelte')).default }));
vi.mock('../bigpicture/components/GamePage.svelte', async () => ({ default: (await import('../testing/BigPictureStorePageStub.svelte')).default }));
vi.mock('../bigpicture/components/DownloadsPage.svelte', async () => ({ default: (await import('../testing/BigPictureStorePageStub.svelte')).default }));
vi.mock('../bigpicture/components/SettingsPage.svelte', async () => ({ default: (await import('../testing/BigPictureStorePageStub.svelte')).default }));
vi.mock('../bigpicture/components/ProfilePage.svelte', async () => ({ default: (await import('../testing/BigPictureStorePageStub.svelte')).default }));
vi.mock('../bigpicture/components/SocialPage.svelte', async () => ({ default: (await import('../testing/BigPictureStorePageStub.svelte')).default }));

let app: HTMLDivElement;
let pad: { id: string; index: number; mapping: string; connected: boolean; axes: number[]; buttons: { pressed: boolean; value: number }[] };

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  // Big Picture navigates only among controls that have a box; jsdom lays nothing out.
  // The pad is polled every frame only while the window has focus; jsdom's first test has none yet.
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  vi.spyOn(Element.prototype, 'getClientRects').mockImplementation(() => [{}] as unknown as DOMRectList);
  // One row, left to right in page order: Play, then the store links.
  const column = ['game:play', 'game:store:steam', 'game:store:gog'];
  vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
    const left = Math.max(0, column.indexOf(this.getAttribute('data-bp-focus') ?? '')) * 200;
    return { left, top: 100, right: left + 160, bottom: 140, width: 160, height: 40, x: left, y: 100, toJSON: () => ({}) } as DOMRect;
  });
  pad = { id: 'Xbox Wireless Controller', index: 0, mapping: 'standard', connected: true, axes: [0, 0, 0, 0], buttons: Array.from({ length: 16 }, () => ({ pressed: false, value: 0 })) };
  Object.defineProperty(navigator, 'getGamepads', { value: () => [pad], configurable: true });
});

afterEach(() => {
  app.remove();
  vi.restoreAllMocks();
  Reflect.deleteProperty(navigator, 'getGamepads');
});

async function press(button: number) {
  pad.buttons[button] = { pressed: true, value: 1 };
  await new Promise((resolve) => requestAnimationFrame(resolve));
  await new Promise((resolve) => requestAnimationFrame(resolve));
  pad.buttons[button] = { pressed: false, value: 0 };
  await new Promise((resolve) => requestAnimationFrame(resolve));
  await new Promise((resolve) => requestAnimationFrame(resolve));
}

const RIGHT = 15;
const A = 0;

async function reachSteam() {
  render(BigPictureShell, { target: app });
  const play = await screen.findByRole('button', { name: 'Play' });
  await vi.waitFor(() => expect(document.activeElement).toBe(play));
  const steam = screen.getByRole('link', { name: msg('games.storeSteam') });
  for (let step = 0; step < 4 && document.activeElement !== steam; step += 1) await press(RIGHT);
  return steam;
}

describe('Big Picture: a store page link on the game page', () => {
  it('is reached with the D-pad and opened by the launcher with A, once', async () => {
    const steam = await reachSteam();
    expect(document.activeElement).toBe(steam);

    await press(A);

    expect(document.querySelector('output')?.textContent).toBe('steam;');
  });

  it('is opened by the launcher with Enter, once', async () => {
    const steam = await reachSteam();
    expect(document.activeElement).toBe(steam);

    await user().keyboard('{Enter}');

    expect(document.querySelector('output')?.textContent).toBe('steam;');
  });
});
