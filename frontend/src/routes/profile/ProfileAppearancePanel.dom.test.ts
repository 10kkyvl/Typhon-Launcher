import { screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/stores/profileAccent', async (actual) => ({
  ...(await actual<typeof import('../../lib/stores/profileAccent')>()),
  launcherAccent: vi.fn(() => null),
}));

import { get } from 'svelte/store';
import { locale } from '../../lib/i18n';
import { artStatus } from '../../lib/profile/artPalette';
import { DEFAULT_PROFILE, type CurrentUser } from '../../lib/services/account';
import { renderDom } from '../../lib/testing/dom';
import { launcherAccent } from '../../lib/stores/profileAccent';
import { authState, currentUser } from '../../lib/stores/user';
import { toasts } from '../../lib/stores/toasts';
import Profile from './Profile.svelte';

// The profile page of a signed-in user, in its editor: the appearance panel is the side panel while no
// block is selected.
const me: CurrentUser = {
  id: 'u1',
  username: 'ann',
  displayName: 'Ann',
  email: 'ann@example.com',
  avatarUrl: '',
  bio: '',
  profile: DEFAULT_PROFILE,
  createdAt: '2025-01-01T00:00:00Z',
};

beforeEach(() => {
  locale.set('en');
  authState.set('authenticated');
  currentUser.set(me);
});

afterEach(() => {
  currentUser.set(null);
  authState.set('bootstrapping');
  toasts.set([]);
  document.body.replaceChildren();
});

async function editor() {
  const view = renderDom(Profile);
  await view.user.click(screen.getByRole('button', { name: 'Customize profile' }));
  return view;
}

const radios = (group: HTMLElement) => within(group).getAllByRole('radio');
const checked = (group: HTMLElement) => radios(group).filter((radio) => radio.getAttribute('aria-checked') === 'true');
// The radios a Tab reaches: a radio group is one stop, on its chosen option.
const stops = (group: HTMLElement) => radios(group).filter((radio) => radio.tabIndex >= 0);

describe('the tile and swatch pickers of the profile appearance are radio groups', () => {
  it('the theme is one choice of a named group: one Tab stop on the chosen tile, the arrows choose', async () => {
    const { user } = await editor();
    const group = screen.getByRole('radiogroup', { name: 'Profile theme' });
    expect(radios(group).map((radio) => radio.textContent?.trim())).toContain('Custom gradient');
    expect(checked(group).map((radio) => radio.textContent?.trim())).toEqual(['Midnight']);
    expect(stops(group)).toEqual(checked(group));
    expect(within(group).queryAllByRole('button')).toEqual([]);

    checked(group)[0].focus();
    await user.keyboard('{ArrowRight}');

    const now = checked(group);
    expect(now.map((radio) => radio.textContent?.trim())).not.toEqual(['Midnight']);
    expect(document.activeElement).toBe(now[0]);
    expect(stops(group)).toEqual(now);
  });

  it('the accent swatches, the frames and the name styles are radio groups named by their headings', async () => {
    const { user } = await editor();
    const swatches = screen.getByRole('radiogroup', { name: 'Own color' });
    expect(radios(swatches)[0].getAttribute('aria-label')).toMatch(/^Accent #/);
    expect(stops(swatches)).toHaveLength(1);

    const frames = screen.getByRole('radiogroup', { name: 'Avatar frame' });
    expect(checked(frames)).toEqual([within(frames).getByRole('radio', { name: 'No frame' })]);
    await user.click(within(frames).getByRole('radio', { name: 'Ring' }));
    expect(checked(frames)).toEqual([within(frames).getByRole('radio', { name: 'Ring' })]);

    const names = screen.getByRole('radiogroup', { name: 'Name style' });
    expect(radios(names).map((radio) => radio.getAttribute('aria-checked'))).toEqual(['true', 'false', 'false']);
    expect(within(names).getByRole('radio', { name: 'Plain' })).toBe(checked(names)[0]);
    expect(stops(names)).toEqual(checked(names));
    checked(names)[0].focus();
    await user.keyboard('{End}');
    expect(document.activeElement).toBe(radios(names).at(-1));
    expect(checked(names)).toEqual([radios(names).at(-1)]);
  });

  it('the accent source is a segmented choice; a launcher accent that cannot be read leaves "Own color" chosen', async () => {
    vi.mocked(launcherAccent).mockReturnValue(null);
    const { user } = await editor();
    const source = screen.getByRole('radiogroup', { name: 'Accent' });
    const own = within(source).getByRole('radio', { name: 'Own color' });
    const launcher = within(source).getByRole('radio', { name: 'Same as launcher theme' });
    expect(own.getAttribute('aria-checked')).toBe('true');

    await user.click(launcher);

    expect(own.getAttribute('aria-checked')).toBe('true');
    expect(get(toasts).map((toast) => toast.message)).toContain('Could not read the accent from the theme.');

    vi.mocked(launcherAccent).mockReturnValue('#729bff');
    await user.click(launcher);

    expect(launcher.getAttribute('aria-checked')).toBe('true');
  });

  it('in every tile and swatch group the option an arrow, Home or End lands on is the one chosen and the one Tab stop', async () => {
    const { user } = await editor();
    const keys = ['{ArrowRight}', '{ArrowRight}', '{ArrowLeft}', '{End}', '{Home}', '{ArrowDown}'];
    const roamed: string[] = [];
    async function arrowsChoose(name: string) {
      const group = () => screen.getByRole('radiogroup', { name });
      stops(group())[0].focus();
      for (const key of keys) {
        await user.keyboard(key);
        const active = document.activeElement as HTMLElement;
        expect(radios(group()), `${name} ${key}`).toContain(active);
        expect(checked(group()), `${name} ${key}`).toEqual([active]);
        expect(stops(group()), `${name} ${key}`).toEqual([active]);
      }
      roamed.push(name);
    }

    await arrowsChoose('Profile theme');
    await arrowsChoose('Own color');
    await arrowsChoose('Avatar frame');
    await arrowsChoose('Name style');
    await user.click(screen.getByRole('button', { name: 'Pick an emoji' }));
    await arrowsChoose('Pick an emoji');

    expect(roamed).toHaveLength(5);
    const chosen = checked(screen.getByRole('radiogroup', { name: 'Pick an emoji' }))[0].textContent;
    expect(screen.getByRole('button', { name: 'Pick an emoji' }).textContent?.trim()).toBe(chosen);
  });

  it('the source of the auto theme is a segmented choice of its own name', async () => {
    const { user } = await editor();
    await user.click(screen.getByRole('radio', { name: 'Auto from game' }));

    const source = screen.getByRole('radiogroup', { name: 'Game to style from' });
    const chosen = within(source).getAllByRole('radio').filter((radio) => radio.getAttribute('aria-checked') === 'true');
    expect(chosen).toHaveLength(1);
    chosen[0].focus();

    await user.keyboard('{ArrowRight}');

    expect(document.activeElement).not.toBe(chosen[0]);
    expect(document.activeElement?.getAttribute('aria-checked')).toBe('true');
  });
});

describe('the disclosures of the profile appearance', () => {
  it('the emoji button shows the emoji choices it controls; a pick closes them and focus goes back to it', async () => {
    const { user } = await editor();
    const opener = screen.getByRole('button', { name: 'Pick an emoji' });
    expect(opener.classList.contains('icon-btn')).toBe(true);
    expect(opener.getAttribute('aria-expanded')).toBe('false');

    opener.focus();
    await user.keyboard('{Enter}');

    const grid = screen.getByRole('radiogroup', { name: 'Pick an emoji' });
    expect(opener.getAttribute('aria-expanded')).toBe('true');
    expect(opener.getAttribute('aria-controls')).toBe(grid.id);
    expect(stops(grid)).toHaveLength(1);

    stops(grid)[0].focus();
    await user.keyboard('{ArrowRight}');
    expect(screen.getByRole('radiogroup', { name: 'Pick an emoji' })).toBe(grid);
    await user.keyboard('{Enter}');

    expect(screen.queryByRole('radiogroup', { name: 'Pick an emoji' })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Pick an emoji' }));
    expect(screen.getByRole('button', { name: 'Remove emoji' })).toBeTruthy();
  });

  it('the custom accent button and the colour swatches say which palette they show', async () => {
    const { user } = await editor();
    const custom = screen.getByRole('button', { name: 'Custom' });
    expect(custom.getAttribute('aria-expanded')).toBe('false');

    await user.click(custom);

    expect(custom.getAttribute('aria-expanded')).toBe('true');
    expect(document.getElementById(custom.getAttribute('aria-controls') ?? '')).not.toBeNull();

    const name = screen.getByRole('button', { name: 'Name color' });
    await user.click(name);
    expect(name.getAttribute('aria-expanded')).toBe('true');
    expect(document.getElementById(name.getAttribute('aria-controls') ?? '')).not.toBeNull();
    expect(custom.getAttribute('aria-expanded')).toBe('false');
    expect(custom.hasAttribute('aria-controls')).toBe(false);
  });
});

const coverInput = () => document.querySelector<HTMLInputElement>('input[type="file"]') as HTMLInputElement;

describe('the messages of the profile appearance', () => {
  it('a status with a refused character marks its field, drawn as every field, invalid and tied to its alert', async () => {
    const { user } = await editor();
    const field = screen.getByRole('textbox', { name: 'Status' });
    expect(field.classList.contains('input')).toBe(true);
    expect(field.getAttribute('aria-invalid')).not.toBe('true');

    await user.click(field);
    await user.paste('gg‮');

    expect(field.getAttribute('aria-invalid')).toBe('true');
    const error = document.getElementById(field.getAttribute('aria-describedby') ?? '');
    expect(error?.getAttribute('role')).toBe('alert');
    expect(error?.textContent).toMatch(/^The text has characters that are not allowed/);
  });

  it('a cover that cannot be used is a danger notice of the cover section', async () => {
    const { user } = await editor();

    await user.upload(coverInput(), new File([], 'empty.png', { type: 'image/png' }));

    const alert = screen.getByRole('alert');
    expect(alert.textContent?.trim()).toBe('Choose a JPG, PNG, WebP or GIF up to 8 MB.');
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });

  it('a cover being read is shown by the spinner, named by what it does', async () => {
    const { user } = await editor();

    await user.upload(coverInput(), new File(['x'], 'cover.png', { type: 'image/png' }));

    await screen.findByText('Uploading cover…');
    const status = screen.getAllByRole('status').filter((element) => element.textContent?.trim() === 'Uploading cover…');
    expect(status).toHaveLength(1);
    expect(status[0].classList.contains('spinner')).toBe(true);
  });

  it('an auto theme whose art could not be read says so in a warning notice', async () => {
    const { user } = await editor();
    await user.click(screen.getByRole('radio', { name: 'Auto from game' }));
    artStatus.set('failed');

    const text = await screen.findByText('Could not take colors from the art — using the accent.');
    expect(text.closest('[data-tone="warning"]')).not.toBeNull();
    artStatus.set('idle');
  });

  it('a worn frame that was revoked is a standing warning of the panel, not announced', async () => {
    const appearance = { ...DEFAULT_PROFILE.appearance, avatarFrame: 'custom:3f2b8c1e-9a4d-4e57-b0aa-1c2d3e4f5a6b' };
    currentUser.set({ ...me, frames: [], profile: { ...DEFAULT_PROFILE, appearance } as CurrentUser['profile'] });
    await editor();

    const notice = screen.getByText('Frame revoked').closest('[data-tone="warning"]') as HTMLElement;
    expect(notice).not.toBeNull();
    expect(notice.querySelector('[role="status"], [role="alert"]')).toBeNull();
  });
});
