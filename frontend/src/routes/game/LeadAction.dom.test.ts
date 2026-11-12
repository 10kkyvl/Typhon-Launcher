import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import type { PrimaryKind } from '../../lib/game/view';
import type { LibraryGame } from '../../lib/services/library';
import { user } from '../../lib/testing/dom';
import LeadAction from './LeadAction.svelte';

type Game = Pick<LibraryGame, 'profiles' | 'lastProfile' | 'emulation'>;

const profile = { id: 'p1', name: 'DirectX 11', executable: 'game-dx11.exe' };
const ONE_PROFILE: Game = { profiles: [profile], lastProfile: '', emulation: null };
const NO_PROFILES: Game = { profiles: [], lastProfile: '', emulation: null };
// An emulated game launches through its emulator: saved profiles do not apply and no chevron is drawn.
const EMULATED: Game = { profiles: [profile], lastProfile: '', emulation: { platform: 'ps2' } as LibraryGame['emulation'] };

let app: HTMLDivElement;

beforeEach(() => {
  locale.set('en');
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
  locale.set('ru');
});

function mount(game: Game | undefined, kind: PrimaryKind = 'play') {
  const before = document.createElement('button');
  before.textContent = 'Before';
  const target = document.createElement('div');
  const after = document.createElement('button');
  after.textContent = 'After';
  app.append(before, target, after);
  const onclick = vi.fn();
  const onpick = vi.fn();
  const children = createRawSnippet(() => ({ render: () => '<span>Play</span>' }));
  render(LeadAction, { target, props: { kind, game, onclick, onpick, children } });
  const play = screen.getByRole('button', { name: 'Play' });
  return { before, after, play, onclick, onpick };
}

const chooser = () => screen.queryByRole('button', { name: msg('games.playProfileChoose') });

describe("the game page's main action", () => {
  it('with a profile besides the default is a group named Play: the action and its chooser, joined, in Tab order', async () => {
    const { before, after, play, onclick } = mount(ONE_PROFILE);
    const group = screen.getByRole('group', { name: msg('games.play') });
    const chevron = chooser();
    expect(chevron).not.toBeNull();
    expect(group.contains(play) && group.contains(chevron)).toBe(true);
    expect(play.classList.contains('join-end')).toBe(true);
    expect(chevron?.classList.contains('join-start')).toBe(true);
    expect(chevron?.classList.contains('icon')).toBe(true);
    expect(chevron?.getAttribute('aria-haspopup')).toBe('menu');

    const keys = user();
    before.focus();
    await keys.tab();
    expect(document.activeElement).toBe(play);
    await keys.tab();
    expect(document.activeElement).toBe(chevron);
    await keys.tab();
    expect(document.activeElement).toBe(after);

    await keys.click(play);
    expect(onclick).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('menu')).toBeNull();
  });

  it('without saved profiles is a plain button: no group, no chevron, no joined side', () => {
    const { play } = mount(NO_PROFILES);
    expect(screen.queryByRole('group')).toBeNull();
    expect(chooser()).toBeNull();
    expect(play.classList.contains('join-end')).toBe(false);
  });

  it('for an emulated game with saved profiles is a plain button, not flattened towards a chevron it does not have', () => {
    const { play } = mount(EMULATED);
    expect(screen.queryByRole('group')).toBeNull();
    expect(chooser()).toBeNull();
    expect(play.classList.contains('join-end')).toBe(false);
  });

  it.each<PrimaryKind>(['stop', 'download', 'install-download', 'retry-download', 'other-versions'])(
    'when the action is %s, offers no profiles even for a game that has them',
    (kind) => {
      const { play } = mount(ONE_PROFILE, kind);
      expect(screen.queryByRole('group')).toBeNull();
      expect(chooser()).toBeNull();
      expect(play.classList.contains('join-end')).toBe(false);
    },
  );

  it('with no game in the library offers no profiles', () => {
    const { play } = mount(undefined);
    expect(screen.queryByRole('group')).toBeNull();
    expect(chooser()).toBeNull();
    expect(play.classList.contains('join-end')).toBe(false);
  });
});
